package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/scanner"
)

type Enforcer struct {
	env   *cel.Env
	rules []cel.Program
	db    db.Store
	mu    sync.RWMutex
}

func NewEnforcer(store db.Store) (*Enforcer, error) {
	// 1. Define the Variables the CEL expressions can evaluate
	env, err := cel.NewEnv(
		cel.Variable("request.method", cel.StringType),
		cel.Variable("request.namespace", cel.StringType),
		cel.Variable("request.repository", cel.StringType),
		cel.Variable("request.reference", cel.StringType), // tag or digest
		cel.Variable("request.signature_is_valid", cel.BoolType),
		cel.Variable("request.signature_issuer", cel.StringType),
		cel.Variable("request.vuln_critical_count", cel.IntType),
		cel.Variable("request.vuln_high_count", cel.IntType),
		ext.Strings(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL env: %v", err)
	}

	enforcer := &Enforcer{
		env:   env,
		rules: []cel.Program{},
		db:    store,
	}

	// Load policies from the database
	if err := enforcer.ReloadPolicies(context.Background()); err != nil {
		log.Printf("[Policy] Warning: Failed to load policies from DB: %v", err)
	}

	return enforcer, nil
}

// ReloadPolicies clears the current rules and reloads them from the database
func (e *Enforcer) ReloadPolicies(ctx context.Context) error {
	policies, err := e.db.ListPolicies(ctx)
	if err != nil {
		return err
	}

	var parsedRules []cel.Program
	for _, p := range policies {
		ast, issues := e.env.Compile(p.Expression)
		if issues != nil && issues.Err() != nil {
			log.Printf("[Policy] Failed to compile policy config ID %d: %v", p.ID, issues.Err())
			continue
		}
		prg, err := e.env.Program(ast)
		if err != nil {
			log.Printf("[Policy] Failed to program policy config ID %d: %v", p.ID, err)
			continue
		}
		parsedRules = append(parsedRules, prg)
	}

	e.mu.Lock()
	e.rules = parsedRules
	e.mu.Unlock()

	return nil
}

// AddRule compiles and adds a raw CEL expression to the database and reloads
func (e *Enforcer) AddRule(ctx context.Context, expression string) error {
	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compile error: %s", issues.Err())
	}

	_, err := e.env.Program(ast)
	if err != nil {
		return fmt.Errorf("program error: %s", err)
	}

	// Persist to DB
	if err := e.db.AddPolicy(ctx, expression); err != nil {
		return fmt.Errorf("failed to persist policy: %v", err)
	}

	return e.ReloadPolicies(ctx)
}

// Protect blocks any OCI Pull/Push if any CEL rule evaluates to false
func (e *Enforcer) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repo := chi.URLParam(r, "repo")
		ns := chi.URLParam(r, "namespace")

		// If accessing a specific resource tag/digest
		ref := chi.URLParam(r, "reference")
		if ref == "" {
			ref = chi.URLParam(r, "digest")
		}
		if ref == "" {
			ref = chi.URLParam(r, "uuid")
		}

		var signatureValid = false
		var signatureIssuer = ""
		var vulnCriticalCount = 0
		var vulnHighCount = 0

		if r.Method == "GET" || r.Method == "HEAD" {
			// Resolve the digest to check for a sig tag and vulnerabilities
			_, digest, _, err := e.db.GetManifest(r.Context(), filepath.Join(ns, repo), ref)
			if err == nil {
				// 1. Signature check
				// Cosign tag format: sha256-<hex>.sig
				sigTag := strings.Replace(digest, ":", "-", 1) + ".sig"
				_, _, _, sigErr := e.db.GetManifest(r.Context(), filepath.Join(ns, repo), sigTag)
				if sigErr == nil {
					signatureValid = true
					signatureIssuer = "cosign" // basic stub for MVP
				}

				// 2. Vulnerability check
				reportData, reportErr := e.db.GetScanReport(r.Context(), digest, "trivy-embedded")
				if reportErr == nil {
					var report scanner.Report
					if parseErr := json.Unmarshal(reportData, &report); parseErr == nil {
						for _, v := range report.Vulnerabilities {
							if v.Severity == "CRITICAL" {
								vulnCriticalCount++
							} else if v.Severity == "HIGH" {
								vulnHighCount++
							}
						}
					}
				}
			}
		}

		// Inject dynamic variables into the CEL runtime
		vars := map[string]interface{}{
			"request.method":              strings.ToUpper(r.Method),
			"request.namespace":           ns,
			"request.repository":          repo,
			"request.reference":           ref,
			"request.signature_is_valid":  signatureValid,
			"request.signature_issuer":    signatureIssuer,
			"request.vuln_critical_count": vulnCriticalCount,
			"request.vuln_high_count":     vulnHighCount,
		}

		e.mu.RLock()
		defer e.mu.RUnlock()

		for i, rule := range e.rules {
			out, _, err := rule.Eval(vars)
			if err != nil {
				log.Printf("CEL Evaluation Error on rule %d: %v", i, err)
				http.Error(w, `{"errors":[{"code":"POLICY_ERROR","message":"internal policy enforcement failure"}]}`, http.StatusInternalServerError)
				return
			}
			if out.Value() == false {
				http.Error(w, `{"errors":[{"code":"DENIED","message":"transaction blocked by registry security policy"}]}`, http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
