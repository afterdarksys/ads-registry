package management

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/automation"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/policy"
	"golang.org/x/crypto/bcrypt"
)

type Router struct {
	db         db.Store
	authMid    *auth.Middleware
	tokenTs    *auth.TokenService
	enforcer   *policy.Enforcer
	starlark   *automation.Engine
	vulnGate   *config.VulnGateConfig
	ldapConfig config.LDAPConfig
	oidcConfig config.OIDCConfig
}

func NewRouter(dbStore db.Store, ts *auth.TokenService, enf *policy.Enforcer, star *automation.Engine, devMode bool) *Router {
	return &Router{
		db:       dbStore,
		tokenTs:  ts,
		authMid:  auth.NewMiddleware(ts, devMode),
		enforcer: enf,
		starlark: star,
	}
}

// WithConfig attaches LDAP and OIDC configuration so that status endpoints
// can expose safe (non-secret) values. Call before Register.
func (r *Router) WithConfig(ldap config.LDAPConfig, oidc config.OIDCConfig) *Router {
	r.ldapConfig = ldap
	r.oidcConfig = oidc
	return r
}

// WithVulnGate attaches the vulnerability gate configuration.
func (r *Router) WithVulnGate(vg *config.VulnGateConfig) *Router {
	r.vulnGate = vg
	return r
}

func (r *Router) Register(mux chi.Router) {
	mux.Route("/api/v1/management", func(api chi.Router) {
		// User-level routes (any authenticated user)
		api.Group(func(userAPI chi.Router) {
			userAPI.Use(r.authMid.Protect)

			// Access Tokens - users can manage their own tokens
			userAPI.Get("/access-tokens", r.listAccessTokens)
			userAPI.Post("/access-tokens", r.createAccessToken)
			userAPI.Delete("/access-tokens/{id}", r.deleteAccessToken)
		})

		// Admin-only routes
		api.Group(func(adminAPI chi.Router) {
			adminAPI.Use(r.authMid.ProtectAdmin)

			adminAPI.Get("/stats", r.getStats)

			// Users
			adminAPI.Get("/users", r.listUsers)
			adminAPI.Post("/users", r.createUser)
			adminAPI.Delete("/users/{username}", r.deleteUser)
			adminAPI.Put("/users/{username}", r.updateUser)
			adminAPI.Post("/users/{username}/reset-password", r.resetPassword)

			// Groups & Quotas
			adminAPI.Get("/quotas", r.listQuotas)
			adminAPI.Post("/quotas", r.setQuota)

			adminAPI.Get("/groups", r.listGroups)
			adminAPI.Post("/groups", r.createGroup)
			adminAPI.Post("/groups/{name}/users", r.addUserToGroup)

			// Repositories
			adminAPI.Get("/repositories", r.listRepositories)
			// FIVE-level repository (register FIRST - most specific)
			adminAPI.Get("/repositories/{org2}/{org1}/{org}/{namespace}/{repo}/tags", r.listTags)
			adminAPI.Get("/repositories/{org2}/{org1}/{org}/{namespace}/{repo}/manifests", r.listManifestsForRepo)

			// FOUR-level repository
			adminAPI.Get("/repositories/{org1}/{org}/{namespace}/{repo}/tags", r.listTags)
			adminAPI.Get("/repositories/{org1}/{org}/{namespace}/{repo}/manifests", r.listManifestsForRepo)

			// THREE-level repository
			adminAPI.Get("/repositories/{org}/{namespace}/{repo}/tags", r.listTags)
			adminAPI.Get("/repositories/{org}/{namespace}/{repo}/manifests", r.listManifestsForRepo)

			// TWO-level repository
			adminAPI.Get("/repositories/{namespace}/{repo}/tags", r.listTags)
			adminAPI.Get("/repositories/{namespace}/{repo}/manifests", r.listManifestsForRepo)

			// SINGLE-level repository (register LAST - least specific)
			adminAPI.Get("/repositories/{repo}/tags", r.listTags)
			adminAPI.Get("/repositories/{repo}/manifests", r.listManifestsForRepo)

			// Upstream Registries
			adminAPI.Get("/upstreams", r.listUpstreams)

			// Vulnerability Scans
			adminAPI.Get("/scans", r.listScans)
			adminAPI.Get("/scans/{digest}", r.getScanReport)

			// Vulnerability Gate
			adminAPI.Get("/vuln-gate", r.getVulnGate)

			// Policies
			adminAPI.Get("/policies", r.listPolicies)
			adminAPI.Post("/policies", r.addPolicy)
			adminAPI.Delete("/policies/{id}", r.deletePolicy)

			// Scripts
			adminAPI.Get("/scripts", r.listScripts)
			adminAPI.Get("/scripts/{name}", r.getScript)
			adminAPI.Put("/scripts/{name}", r.putScript)
			adminAPI.Delete("/scripts/{name}", r.deleteScript)
			adminAPI.Post("/scripts/{name}/enable", r.enableScript)
			adminAPI.Post("/scripts/{name}/disable", r.disableScript)

			// Immutable Tags
			adminAPI.Patch("/repositories/{namespace}/{repo}/tags/{reference}", r.patchTag)
			adminAPI.Patch("/repositories/{org}/{namespace}/{repo}/tags/{reference}", r.patchTag)
			adminAPI.Patch("/repositories/{org1}/{org}/{namespace}/{repo}/tags/{reference}", r.patchTag)
			adminAPI.Patch("/repositories/{org2}/{org1}/{org}/{namespace}/{repo}/tags/{reference}", r.patchTag)

			// LDAP
			adminAPI.Get("/ldap/status", r.getLDAPStatus)
			adminAPI.Post("/ldap/sync", r.postLDAPSync)

			// OIDC/SSO
			adminAPI.Get("/auth/oidc/status", r.getOIDCStatus)
		})

	})
}

// ----------------------------------------------------
// Handlers
// ----------------------------------------------------

func (r *Router) getStats(w http.ResponseWriter, req *http.Request) {
	// Get real stats
	repos, _ := r.db.ListRepositories(req.Context(), 10000, "")
	quotas, _ := r.db.ListQuotas(req.Context())
	scans, _ := r.db.ListScanReports(req.Context())

	// Calculate total storage
	var totalStorage int64
	for _, q := range quotas {
		totalStorage += q.UsedBytes
	}

	// Count critical vulnerabilities
	criticalVulns := 0
	for _, scan := range scans {
		var trivyReport struct {
			Results []struct {
				Vulnerabilities []struct {
					Severity string `json:"Severity"`
				} `json:"Vulnerabilities"`
			} `json:"Results"`
		}
		if err := json.Unmarshal(scan.Data, &trivyReport); err == nil {
			for _, result := range trivyReport.Results {
				for _, vuln := range result.Vulnerabilities {
					if vuln.Severity == "CRITICAL" {
						criticalVulns++
					}
				}
			}
		}
	}

	// Format storage size
	storageStr := formatBytes(totalStorage)

	policies, _ := r.db.ListPolicies(req.Context())

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_repos":    len(repos),
		"storage_used":   storageStr,
		"critical_vulns": criticalVulns,
		"policy_blocks":  len(policies),
	})
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (r *Router) listUsers(w http.ResponseWriter, req *http.Request) {
	users, err := r.db.ListUsers(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Redact password hashes before sending
	for i := range users {
		users[i].PasswordHash = ""
	}
	json.NewEncoder(w).Encode(users)
}

func (r *Router) createUser(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Username string   `json:"username"`
		Password string   `json:"password"`
		Scopes   []string `json:"scopes"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	if err := r.db.CreateUser(req.Context(), payload.Username, string(hashed), payload.Scopes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (r *Router) deleteUser(w http.ResponseWriter, req *http.Request) {
	username := chi.URLParam(req, "username")
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}

	if err := r.db.DeleteUser(req.Context(), username); err != nil {
		if err == db.ErrNotFound {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) updateUser(w http.ResponseWriter, req *http.Request) {
	username := chi.URLParam(req, "username")
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}

	var payload struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := r.db.UpdateUser(req.Context(), username, payload.Scopes); err != nil {
		if err == db.ErrNotFound {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (r *Router) resetPassword(w http.ResponseWriter, req *http.Request) {
	username := chi.URLParam(req, "username")
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}

	var payload struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(payload.Password) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	if err := r.db.UpdateUserPassword(req.Context(), username, string(hashed)); err != nil {
		if err == db.ErrNotFound {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (r *Router) listQuotas(w http.ResponseWriter, req *http.Request) {
	quotas, err := r.db.ListQuotas(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(quotas)
}

func (r *Router) setQuota(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Namespace  string `json:"namespace"`
		LimitBytes int64  `json:"limit_bytes"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if err := r.db.SetQuota(req.Context(), payload.Namespace, payload.LimitBytes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (r *Router) listGroups(w http.ResponseWriter, req *http.Request) {
	groups, err := r.db.ListGroups(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(groups)
}

func (r *Router) createGroup(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := r.db.CreateGroup(req.Context(), payload.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (r *Router) addUserToGroup(w http.ResponseWriter, req *http.Request) {
	groupName := chi.URLParam(req, "name")
	var payload struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := r.db.AddUserToGroup(req.Context(), payload.Username, groupName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (r *Router) listRepositories(w http.ResponseWriter, req *http.Request) {
	repos, err := r.db.ListRepositories(req.Context(), 100, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(repos)
}

func (r *Router) listUpstreams(w http.ResponseWriter, req *http.Request) {
	upstreams, err := r.db.ListUpstreams(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(upstreams)
}

func (r *Router) listTags(w http.ResponseWriter, req *http.Request) {
	// Extract repo from URL path: /api/v1/management/repositories/{repo}/tags
	repo := strings.TrimPrefix(req.URL.Path, "/api/v1/management/repositories/")
	repo = strings.TrimSuffix(repo, "/tags")
	if repo == "" {
		http.Error(w, "repo parameter is required", http.StatusBadRequest)
		return
	}

	tags, err := r.db.ListTags(req.Context(), repo, 1000, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(tags)
}

func (r *Router) listManifestsForRepo(w http.ResponseWriter, req *http.Request) {
	// Extract repo from URL path: /api/v1/management/repositories/{repo}/manifests
	repo := strings.TrimPrefix(req.URL.Path, "/api/v1/management/repositories/")
	repo = strings.TrimSuffix(repo, "/manifests")
	if repo == "" {
		http.Error(w, "repo parameter is required", http.StatusBadRequest)
		return
	}

	// Get all manifests and filter by repo
	manifests, err := r.db.ListManifests(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var filtered []db.ManifestRecord
	for _, m := range manifests {
		fullRepo := m.Namespace + "/" + m.Repo
		if fullRepo == repo || m.Repo == repo {
			filtered = append(filtered, m)
		}
	}

	json.NewEncoder(w).Encode(filtered)
}

func (r *Router) listScans(w http.ResponseWriter, req *http.Request) {
	reports, err := r.db.ListScanReports(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse DarkScan reports and return them directly
	// The frontend expects the full DarkScan format
	var scanResults []map[string]interface{}
	for _, report := range reports {
		if report.Scanner != "darkscan" {
			continue // Only return DarkScan results
		}

		var scanData map[string]interface{}
		if err := json.Unmarshal(report.Data, &scanData); err != nil {
			log.Printf("[SCANS API] Failed to parse scan report for %s: %v", report.Digest, err)
			continue
		}

		scanResults = append(scanResults, scanData)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scanResults)
}

func (r *Router) getScanReport(w http.ResponseWriter, req *http.Request) {
	digest := chi.URLParam(req, "digest")
	scanner := req.URL.Query().Get("scanner")
	if scanner == "" {
		scanner = "trivy"
	}

	data, err := r.db.GetScanReport(req.Context(), digest, scanner)
	if err != nil {
		if err == db.ErrNotFound {
			http.Error(w, "scan report not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (r *Router) listPolicies(w http.ResponseWriter, req *http.Request) {
	policies, err := r.db.ListPolicies(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(policies)
}

func (r *Router) addPolicy(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := r.enforcer.AddRule(req.Context(), payload.Expression); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (r *Router) deletePolicy(w http.ResponseWriter, req *http.Request) {
	idStr := chi.URLParam(req, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.db.DeletePolicy(req.Context(), id); err != nil {
		if err == db.ErrNotFound {
			http.Error(w, "policy not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload the enforcer rules memory
	r.enforcer.ReloadPolicies(req.Context())

	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) listScripts(w http.ResponseWriter, req *http.Request) {
	files, err := os.ReadDir("scripts")
	if err != nil {
		if os.IsNotExist(err) {
			json.NewEncoder(w).Encode([]map[string]interface{}{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var scripts []map[string]interface{}
	for _, f := range files {
		if !f.IsDir() {
			name := f.Name()
			if strings.HasSuffix(name, ".star") {
				scripts = append(scripts, map[string]interface{}{
					"name":    name,
					"enabled": true,
				})
			} else if strings.HasSuffix(name, ".star.disabled") {
				originalName := strings.TrimSuffix(name, ".disabled")
				scripts = append(scripts, map[string]interface{}{
					"name":    originalName,
					"enabled": false,
				})
			}
		}
	}
	json.NewEncoder(w).Encode(scripts)
}

func (r *Router) getScript(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		http.Error(w, "invalid script name", http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(filepath.Join("scripts", name))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "text/plain")
	w.Write(content)
}

func (r *Router) putScript(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		http.Error(w, "invalid script name", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll("scripts", 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	content, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.WriteFile(filepath.Join("scripts", name), content, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (r *Router) enableScript(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		http.Error(w, "invalid script name", http.StatusBadRequest)
		return
	}

	oldPath := filepath.Join("scripts", name+".disabled")
	newPath := filepath.Join("scripts", name)

	if err := os.Rename(oldPath, newPath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "script is already enabled or does not exist", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (r *Router) disableScript(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		http.Error(w, "invalid script name", http.StatusBadRequest)
		return
	}

	oldPath := filepath.Join("scripts", name)
	newPath := filepath.Join("scripts", name+".disabled")

	if err := os.Rename(oldPath, newPath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "script is already disabled or does not exist", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (r *Router) deleteScript(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		http.Error(w, "invalid script name", http.StatusBadRequest)
		return
	}

	err := os.Remove(filepath.Join("scripts", name))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --------------------------------------------------------------------------------
// Access Tokens (for Docker CLI when using OAuth2/SSO)
// --------------------------------------------------------------------------------

func (r *Router) listAccessTokens(w http.ResponseWriter, req *http.Request) {
	// Get user from auth context
	username := req.Context().Value("username").(string)
	user, err := r.db.GetUserByUsername(req.Context(), username)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	tokens, err := r.db.ListAccessTokens(req.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Don't return the token_hash in the response
	response := make([]map[string]interface{}, len(tokens))
	for i, t := range tokens {
		response[i] = map[string]interface{}{
			"id":           t.ID,
			"name":         t.Name,
			"scopes":       t.Scopes,
			"created_at":   t.CreatedAt,
			"last_used_at": t.LastUsedAt,
			"expires_at":   t.ExpiresAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type createAccessTokenRequest struct {
	Name      string `json:"name"`       // User-friendly name/slug
	ExpiresIn int    `json:"expires_in"` // Optional: days until expiry (0 = no expiry)
}

func (r *Router) createAccessToken(w http.ResponseWriter, req *http.Request) {
	var input createAccessTokenRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if input.Name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	// Get user from auth context
	username := req.Context().Value("username").(string)
	user, err := r.db.GetUserByUsername(req.Context(), username)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Generate random token (32 bytes = 64 hex chars)
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		http.Error(w, "Failed to generate random token", http.StatusInternalServerError)
		return
	}
	token := fmt.Sprintf("adsr_%x", tokenBytes)

	// Hash the token for storage
	tokenHash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Calculate expiry
	var expiresAt *time.Time
	if input.ExpiresIn > 0 {
		exp := time.Now().AddDate(0, 0, input.ExpiresIn)
		expiresAt = &exp
	}

	// Inherit user's scopes
	tokenID, err := r.db.CreateAccessToken(req.Context(), user.ID, input.Name, string(tokenHash), user.Scopes, expiresAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			http.Error(w, "A token with this name already exists", http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Return the token ONCE (can't be retrieved again)
	response := map[string]interface{}{
		"id":         tokenID,
		"name":       input.Name,
		"token":      token,
		"expires_at": expiresAt,
		"docker_login": map[string]string{
			"registry": "registry.afterdarksys.com",
			"username": fmt.Sprintf("%s-oci", user.Username),
			"password": token,
			"command":  fmt.Sprintf("docker login registry.afterdarksys.com -u %s-oci -p %s", user.Username, token),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (r *Router) deleteAccessToken(w http.ResponseWriter, req *http.Request) {
	tokenIDStr := chi.URLParam(req, "id")
	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		http.Error(w, "Invalid token ID", http.StatusBadRequest)
		return
	}

	// Get user from auth context
	username := req.Context().Value("username").(string)
	user, err := r.db.GetUserByUsername(req.Context(), username)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Verify token belongs to user (prevent users from deleting others' tokens)
	tokens, err := r.db.ListAccessTokens(req.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	found := false
	for _, t := range tokens {
		if t.ID == tokenID {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Token not found or does not belong to you", http.StatusNotFound)
		return
	}

	// Delete the token
	if err := r.db.DeleteAccessToken(req.Context(), tokenID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// patchTag handles PATCH /api/v1/management/repositories/{...}/tags/{reference}
// Body: {"immutable": true} or {"immutable": false}
func (r *Router) patchTag(w http.ResponseWriter, req *http.Request) {
	// Extract repo from URL path by stripping prefix and "/tags/{reference}" suffix
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/management/repositories/")
	idx := strings.LastIndex(path, "/tags/")
	if idx < 0 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	repo := path[:idx]
	reference := path[idx+len("/tags/"):]

	if repo == "" || reference == "" {
		http.Error(w, "repo and reference are required", http.StatusBadRequest)
		return
	}

	var payload struct {
		Immutable bool `json:"immutable"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := r.db.SetTagImmutable(req.Context(), repo, reference, payload.Immutable); err != nil {
		if err == db.ErrNotFound {
			http.Error(w, "tag not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"repo":      repo,
		"reference": reference,
		"immutable": payload.Immutable,
	})
}

// getVulnGate returns the current vulnerability gate configuration.
// GET /api/v1/management/vuln-gate
func (r *Router) getVulnGate(w http.ResponseWriter, req *http.Request) {
	if r.vulnGate == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":          false,
			"block_severities": []string{},
			"allow_unscanned":  true,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":          r.vulnGate.Enabled,
		"block_severities": r.vulnGate.BlockSeverities,
		"allow_unscanned":  r.vulnGate.AllowUnscanned,
	})
}

// getLDAPStatus returns safe (non-secret) LDAP configuration.
// GET /api/v1/management/ldap/status
func (r *Router) getLDAPStatus(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":  r.ldapConfig.Enabled,
		"server":   r.ldapConfig.Server,
		"use_ssl":  r.ldapConfig.UseSSL,
		"base_dn":  r.ldapConfig.BaseDN,
	})
}

// postLDAPSync triggers an on-demand LDAP group sync for all users.
// POST /api/v1/management/ldap/sync
func (r *Router) postLDAPSync(w http.ResponseWriter, req *http.Request) {
	if !r.ldapConfig.Enabled {
		http.Error(w, `{"error":"LDAP is not enabled"}`, http.StatusServiceUnavailable)
		return
	}

	// List all users and check which ones appear to be LDAP-provisioned.
	// LDAP users are identified by having an empty password hash (set during
	// auto-provisioning in the auth handler).
	users, err := r.db.ListUsers(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ldapUserCount := 0
	for _, u := range users {
		if u.PasswordHash == "" || u.PasswordHash == "ldap_managed_user" {
			ldapUserCount++
		}
	}

	log.Printf("[LDAP SYNC] on-demand sync triggered by admin; %d potential LDAP users found", ldapUserCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":          "LDAP sync triggered — users will get updated scopes on next login",
		"ldap_user_count":  ldapUserCount,
	})
}

// getOIDCStatus returns safe (non-secret) OIDC configuration.
// GET /api/v1/management/auth/oidc/status
func (r *Router) getOIDCStatus(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":   r.oidcConfig.Enabled,
		"issuer":    r.oidcConfig.Issuer,
		"client_id": r.oidcConfig.ClientID,
	})
}
