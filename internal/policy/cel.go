package policy

import (
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

	return &Enforcer{
		env:   env,
		rules: []cel.Program{},
		db:    store,
	}, nil
}

// AddRule compiles and adds a raw CEL expression (e.g., `request.namespace == "trusted"`)
func (e *Enforcer) AddRule(expression string) error {
	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compile error: %s", issues.Err())
	}

	prg, err := e.env.Program(ast)
	if err != nil {
		return fmt.Errorf("program error: %s", err)
	}

	e.mu.Lock()
	e.rules = append(e.rules, prg)
	e.mu.Unlock()

	return nil
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
