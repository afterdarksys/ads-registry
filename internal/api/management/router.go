package management

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/automation"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/policy"
	"golang.org/x/crypto/bcrypt"
)

type Router struct {
	db       db.Store
	authMid  *auth.Middleware
	tokenTs  *auth.TokenService
	enforcer *policy.Enforcer
	starlark *automation.Engine
}

func NewRouter(dbStore db.Store, ts *auth.TokenService, enf *policy.Enforcer, star *automation.Engine) *Router {
	return &Router{
		db:       dbStore,
		tokenTs:  ts,
		authMid:  auth.NewMiddleware(ts),
		enforcer: enf,
		starlark: star,
	}
}

func (r *Router) Register(mux chi.Router) {
	mux.Route("/api/v1/management", func(api chi.Router) {
		// Protected by admin-level authentication
		api.Use(r.authMid.ProtectAdmin)

		api.Get("/stats", r.getStats)

		// Users
		api.Get("/users", r.listUsers)
		api.Post("/users", r.createUser)
		api.Delete("/users/{username}", r.deleteUser)
		api.Put("/users/{username}", r.updateUser)
		api.Post("/users/{username}/reset-password", r.resetPassword)

		// Groups & Quotas
		api.Get("/quotas", r.listQuotas)
		api.Post("/quotas", r.setQuota)
		
		api.Get("/groups", r.listGroups)
		api.Post("/groups", r.createGroup)
		api.Post("/groups/{name}/users", r.addUserToGroup)

		// Repositories
		api.Get("/repositories", r.listRepositories)
		api.Get("/repositories/{repo}/tags", r.listTags)
		api.Get("/repositories/{repo}/manifests", r.listManifestsForRepo)

		// Upstream Registries
		api.Get("/upstreams", r.listUpstreams)

		// Vulnerability Scans
		api.Get("/scans", r.listScans)
		api.Get("/scans/{digest}", r.getScanReport)

		// Policies
		api.Get("/policies", r.listPolicies)
		api.Post("/policies", r.addPolicy)

		// Scripts
		api.Get("/scripts", r.listScripts)
		api.Get("/scripts/{name}", r.getScript)
		api.Put("/scripts/{name}", r.putScript)
		api.Delete("/scripts/{name}", r.deleteScript)

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

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_repos":      len(repos),
		"storage_used":     storageStr,
		"critical_vulns":   criticalVulns,
		"policy_blocks":    0, // TODO: Add policy block tracking
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
	repo := chi.URLParam(req, "repo")
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
	repo := chi.URLParam(req, "repo")
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

	// Parse scan data and return summary
	type ScanSummary struct {
		Digest   string `json:"digest"`
		Scanner  string `json:"scanner"`
		Critical int    `json:"critical"`
		High     int    `json:"high"`
		Medium   int    `json:"medium"`
		Low      int    `json:"low"`
	}

	var summaries []ScanSummary
	for _, report := range reports {
		// Parse Trivy JSON output
		var trivyReport struct {
			Results []struct {
				Vulnerabilities []struct {
					Severity string `json:"Severity"`
				} `json:"Vulnerabilities"`
			} `json:"Results"`
		}

		summary := ScanSummary{
			Digest:  report.Digest,
			Scanner: report.Scanner,
		}

		if err := json.Unmarshal(report.Data, &trivyReport); err == nil {
			for _, result := range trivyReport.Results {
				for _, vuln := range result.Vulnerabilities {
					switch vuln.Severity {
					case "CRITICAL":
						summary.Critical++
					case "HIGH":
						summary.High++
					case "MEDIUM":
						summary.Medium++
					case "LOW":
						summary.Low++
					}
				}
			}
		}

		summaries = append(summaries, summary)
	}

	json.NewEncoder(w).Encode(summaries)
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
	// Dummy for now until DB table connects
	json.NewEncoder(w).Encode([]map[string]string{})
}

func (r *Router) addPolicy(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := r.enforcer.AddRule(payload.Expression); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (r *Router) listScripts(w http.ResponseWriter, req *http.Request) {
	files, err := os.ReadDir("scripts")
	if err != nil {
		if os.IsNotExist(err) {
			json.NewEncoder(w).Encode([]string{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var scripts []string
	for _, f := range files {
		if !f.IsDir() && filepath.Ext(f.Name()) == ".star" {
			scripts = append(scripts, f.Name())
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
