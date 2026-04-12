package v2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/automation"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/events"
	"github.com/ryan/ads-registry/internal/policy"
	"github.com/ryan/ads-registry/internal/proxy"
	"github.com/ryan/ads-registry/internal/scanner"
	"github.com/ryan/ads-registry/internal/storage"
	internalsync "github.com/ryan/ads-registry/internal/sync"
	"github.com/ryan/ads-registry/internal/upstreams"
	"github.com/ryan/ads-registry/internal/webhooks"
	"golang.org/x/sync/singleflight"
)

type Router struct {
	db            db.Store
	storage       storage.Provider
	authMid       *auth.Middleware
	tokenTs       *auth.TokenService
	enforcer      *policy.Enforcer
	starlark      *automation.Engine
	upstreamProxy *proxy.UpstreamProxy
	syncManager   *internalsync.Manager
	scanner       *scanner.Service
	webhook       *webhooks.Dispatcher
	broker        *events.Broker
	developerMode bool
	ldapClient    *auth.LDAPClient
	vulnGate      *config.VulnGateConfig
	// blobGroup deduplicates concurrent finalizations of the same blob digest,
	// preventing race conditions when multiple clients push identical layers.
	blobGroup singleflight.Group
	// uploadMu serializes concurrent PATCH requests for the same upload UUID,
	// preventing data corruption when two clients append to the same temp file.
	uploadMu sync.Map
}

// lockUpload acquires a per-upload mutex for the given UUID and returns an
// unlock function. Callers must defer the returned function.
func (r *Router) lockUpload(uuid string) func() {
	mu, _ := r.uploadMu.LoadOrStore(uuid, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	return func() { mu.(*sync.Mutex).Unlock() }
}

func (r *Router) SetWebhookDispatcher(wd *webhooks.Dispatcher) {
	r.webhook = wd
}

func (r *Router) SetEventBroker(b *events.Broker) {
	r.broker = b
}

// AuthMiddleware returns the authentication middleware handler
func (r *Router) AuthMiddleware() func(http.Handler) http.Handler {
	return r.authMid.Protect
}

func NewRouter(dbStore db.Store, storageProvider storage.Provider, ts *auth.TokenService, enf *policy.Enforcer, star *automation.Engine, upstreamMgr *upstreams.Manager, syncMgr *internalsync.Manager, scannerSvc *scanner.Service, devMode bool, ldapClient *auth.LDAPClient, vulnGate *config.VulnGateConfig) *Router {
	var upstreamProxy *proxy.UpstreamProxy
	if upstreamMgr != nil {
		upstreamProxy = proxy.NewUpstreamProxy(upstreamMgr)
	}

	return &Router{
		db:            dbStore,
		storage:       storageProvider,
		tokenTs:       ts,
		authMid:       auth.NewMiddleware(ts, devMode),
		enforcer:      enf,
		scanner:       scannerSvc,
		starlark:      star,
		upstreamProxy: upstreamProxy,
		syncManager:   syncMgr,
		developerMode: devMode,
		ldapClient:    ldapClient,
		vulnGate:      vulnGate,
	}
}

// checkVulnGate inspects scan reports for the given digest and returns an error
// if any vulnerability matches the configured block list.
func (r *Router) checkVulnGate(ctx context.Context, digest string) error {
	if r.vulnGate == nil || !r.vulnGate.Enabled {
		return nil
	}

	// Try trivy first, then any scanner
	data, err := r.db.GetScanReport(ctx, digest, "trivy")
	if err != nil {
		// No scan report found
		if r.vulnGate.AllowUnscanned {
			return nil
		}
		return fmt.Errorf("image has not been scanned")
	}

	// Unmarshal the report
	var report scanner.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil // corrupt report — allow pull
	}

	blockSet := make(map[string]bool)
	for _, s := range r.vulnGate.BlockSeverities {
		blockSet[strings.ToUpper(s)] = true
	}

	for _, v := range report.Vulnerabilities {
		if blockSet[strings.ToUpper(v.Severity)] {
			return fmt.Errorf("image blocked: contains %s vulnerability %s in %s", v.Severity, v.ID, v.Package)
		}
	}
	return nil
}

func (r *Router) Register(mux chi.Router) {

	// The Auth Endpoint
	authHandler := auth.NewHandler(r.tokenTs, r.db, r.ldapClient)
	authHandler.Register(mux)

	// API Endpoints
	mux.Route("/v2", func(api chi.Router) {
		// Set Docker Registry API version header on ALL /v2 responses
		api.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
				next.ServeHTTP(w, r)
			})
		})

		// Base check endpoint doesn't need full auth
		api.Get("/", r.baseCheck)

		api.Route("/_catalog", func(catalogCtx chi.Router) {
			catalogCtx.Use(r.authMid.Protect)
			catalogCtx.Get("/", r.getCatalog)
		})

		api.Route("/_events", func(eventsCtx chi.Router) {
			eventsCtx.Use(r.authMid.Protect)
			eventsCtx.Get("/", r.sseEvents)
		})

		// CRITICAL FIX: Register routes in REVERSE order (most specific FIRST)
		// Chi router has a known limitation: when multiple patterns match the same request,
		// it uses the pattern with the MOST path parameters.
		//
		// For example, /v2/tokenworx/blobs/uploads/ would match:
		// - Pattern /{repo}/blobs/uploads/ with repo=tokenworx (1 param) ✓ CORRECT
		// - Pattern /{namespace}/{repo}/blobs/uploads/ with namespace=tokenworx, repo=blobs (2 params) ✗ WRONG
		//
		// Chi would choose the 2-param pattern because it's "more specific", even though it's incorrect.
		//
		// Solution: Register most-specific (most params) routes FIRST, so chi finds them first during traversal

		api.Group(func(repoGroup chi.Router) {
			repoGroup.Use(r.authMid.Protect)

			// DISABLED for initial testing - uncomment to enable policy enforcement
			// if r.enforcer != nil {
			// 	repoGroup.Use(r.enforcer.Protect)
			// }

			// FIVE-level repository (register FIRST - most specific)
			repoGroup.Get("/{org2}/{org1}/{org}/{namespace}/{repo}/tags/list", r.getTags)
			repoGroup.Get("/{org2}/{org1}/{org}/{namespace}/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Head("/{org2}/{org1}/{org}/{namespace}/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Put("/{org2}/{org1}/{org}/{namespace}/{repo}/manifests/{reference}", r.putManifest)
			repoGroup.Delete("/{org2}/{org1}/{org}/{namespace}/{repo}/manifests/{reference}", r.deleteManifest)
			repoGroup.Get("/{org2}/{org1}/{org}/{namespace}/{repo}/referrers/{digest}", r.getReferrers)
			repoGroup.Get("/{org2}/{org1}/{org}/{namespace}/{repo}/blobs/{digest}", r.getBlob)
			repoGroup.Head("/{org2}/{org1}/{org}/{namespace}/{repo}/blobs/{digest}", r.headBlob)
			repoGroup.Post("/{org2}/{org1}/{org}/{namespace}/{repo}/blobs/uploads/", r.startUpload)
			repoGroup.Post("/{org2}/{org1}/{org}/{namespace}/{repo}/blobs/uploads", r.startUpload)
			repoGroup.Get("/{org2}/{org1}/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.getUploadProgress)
			repoGroup.Patch("/{org2}/{org1}/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.patchUpload)
			repoGroup.Put("/{org2}/{org1}/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.putUpload)

			// FOUR-level repository
			repoGroup.Get("/{org1}/{org}/{namespace}/{repo}/tags/list", r.getTags)
			repoGroup.Get("/{org1}/{org}/{namespace}/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Head("/{org1}/{org}/{namespace}/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Put("/{org1}/{org}/{namespace}/{repo}/manifests/{reference}", r.putManifest)
			repoGroup.Delete("/{org1}/{org}/{namespace}/{repo}/manifests/{reference}", r.deleteManifest)
			repoGroup.Get("/{org1}/{org}/{namespace}/{repo}/referrers/{digest}", r.getReferrers)
			repoGroup.Get("/{org1}/{org}/{namespace}/{repo}/blobs/{digest}", r.getBlob)
			repoGroup.Head("/{org1}/{org}/{namespace}/{repo}/blobs/{digest}", r.headBlob)
			repoGroup.Post("/{org1}/{org}/{namespace}/{repo}/blobs/uploads/", r.startUpload)
			repoGroup.Post("/{org1}/{org}/{namespace}/{repo}/blobs/uploads", r.startUpload)
			repoGroup.Get("/{org1}/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.getUploadProgress)
			repoGroup.Patch("/{org1}/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.patchUpload)
			repoGroup.Put("/{org1}/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.putUpload)

			// THREE-level repository
			repoGroup.Get("/{org}/{namespace}/{repo}/tags/list", r.getTags)
			repoGroup.Get("/{org}/{namespace}/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Head("/{org}/{namespace}/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Put("/{org}/{namespace}/{repo}/manifests/{reference}", r.putManifest)
			repoGroup.Delete("/{org}/{namespace}/{repo}/manifests/{reference}", r.deleteManifest)
			repoGroup.Get("/{org}/{namespace}/{repo}/referrers/{digest}", r.getReferrers)
			repoGroup.Get("/{org}/{namespace}/{repo}/blobs/{digest}", r.getBlob)
			repoGroup.Head("/{org}/{namespace}/{repo}/blobs/{digest}", r.headBlob)
			repoGroup.Post("/{org}/{namespace}/{repo}/blobs/uploads/", r.startUpload)
			repoGroup.Post("/{org}/{namespace}/{repo}/blobs/uploads", r.startUpload)
			repoGroup.Get("/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.getUploadProgress)
			repoGroup.Patch("/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.patchUpload)
			repoGroup.Put("/{org}/{namespace}/{repo}/blobs/uploads/{uuid}", r.putUpload)

			// TWO-level repository
			repoGroup.Get("/{namespace}/{repo}/tags/list", r.getTags)
			repoGroup.Get("/{namespace}/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Head("/{namespace}/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Put("/{namespace}/{repo}/manifests/{reference}", r.putManifest)
			repoGroup.Delete("/{namespace}/{repo}/manifests/{reference}", r.deleteManifest)
			repoGroup.Get("/{namespace}/{repo}/referrers/{digest}", r.getReferrers)
			repoGroup.Get("/{namespace}/{repo}/blobs/{digest}", r.getBlob)
			repoGroup.Head("/{namespace}/{repo}/blobs/{digest}", r.headBlob)
			repoGroup.Post("/{namespace}/{repo}/blobs/uploads/", r.startUpload)
			repoGroup.Post("/{namespace}/{repo}/blobs/uploads", r.startUpload)
			repoGroup.Get("/{namespace}/{repo}/blobs/uploads/{uuid}", r.getUploadProgress)
			repoGroup.Patch("/{namespace}/{repo}/blobs/uploads/{uuid}", r.patchUpload)
			repoGroup.Put("/{namespace}/{repo}/blobs/uploads/{uuid}", r.putUpload)

			// SINGLE-level repository (register LAST - least specific)
			repoGroup.Get("/{repo}/tags/list", r.getTags)
			repoGroup.Get("/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Head("/{repo}/manifests/{reference}", r.getManifest)
			repoGroup.Put("/{repo}/manifests/{reference}", r.putManifest)
			repoGroup.Delete("/{repo}/manifests/{reference}", r.deleteManifest)
			repoGroup.Get("/{repo}/referrers/{digest}", r.getReferrers)
			repoGroup.Get("/{repo}/blobs/{digest}", r.getBlob)
			repoGroup.Head("/{repo}/blobs/{digest}", r.headBlob)
			repoGroup.Post("/{repo}/blobs/uploads/", r.startUpload)
			repoGroup.Post("/{repo}/blobs/uploads", r.startUpload)
			repoGroup.Get("/{repo}/blobs/uploads/{uuid}", r.getUploadProgress)
			repoGroup.Patch("/{repo}/blobs/uploads/{uuid}", r.patchUpload)
			repoGroup.Put("/{repo}/blobs/uploads/{uuid}", r.putUpload)
		})
	})
}

func (r *Router) sseEvents(w http.ResponseWriter, req *http.Request) {
	if r.broker == nil {
		http.Error(w, "event streaming not available", http.StatusServiceUnavailable)
		return
	}
	r.broker.ServeSSE(w, req)
}

func (r *Router) baseCheck(w http.ResponseWriter, req *http.Request) {
	if r.developerMode {
		w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
		return
	}

	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")

	// If no auth provided, tell Docker where to get a token
	if req.Header.Get("Authorization") == "" {
		scheme := auth.GetScheme(req)
		w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s://%s/auth/token",service="registry"`, scheme, req.Host))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// getRepoContext extracts the full repository path and namespace for quota/proxy tracking
// It safely strips endpoint suffixes like /blobs/uploads/, /tags/list, etc.
// because Chi route matching might not provide the exact full path securely as a parameter in all scenarios.
func getRepoContext(req *http.Request) (fullRepo string, quotaNs string) {
	// Remove endpoint suffixes
	endpointPrefixes := []string{
		"/blobs/",
		"/manifests/",
		"/tags/",
		"/referrers/",
	}

	cleanPath := req.URL.Path
	for _, prefix := range endpointPrefixes {
		if idx := strings.Index(cleanPath, prefix); idx != -1 {
			cleanPath = cleanPath[:idx]
			break
		}
	}

	// Trim the base /v2/ API version routing prefix to get the full multi-level repo name
	fullRepo = strings.TrimPrefix(cleanPath, "/v2/")

	// Extract the root-most organization/namespace for Quota and Upstream Proxy
	parts := strings.Split(fullRepo, "/")
	if len(parts) >= 2 {
		quotaNs = parts[0]
	} else {
		quotaNs = ""
	}

	return fullRepo, quotaNs
}

func getPath(fullRepo, digest string) string {
	return filepath.Join(fullRepo, digest)
}

func (r *Router) getCatalog(w http.ResponseWriter, req *http.Request) {
	nStr := req.URL.Query().Get("n")
	last := req.URL.Query().Get("last")

	limit := 0
	if nStr != "" {
		parsed, err := strconv.Atoi(nStr)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}

	repos, err := r.db.ListRepositories(req.Context(), limit, last)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if repos == nil {
		repos = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	if limit > 0 && len(repos) == limit {
		lastItem := repos[len(repos)-1]
		w.Header().Set("Link", fmt.Sprintf(`</v2/_catalog?n=%d&last=%s>; rel="next"`, limit, lastItem))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"repositories": repos,
	})
}

func (r *Router) getTags(w http.ResponseWriter, req *http.Request) {
	// Extract repository context from direct URL inspection
	fullRepo, quotaNs := getRepoContext(req)

	nStr := req.URL.Query().Get("n")
	last := req.URL.Query().Get("last")

	// Check if this is an upstream registry request
	upstreamNs := quotaNs
	if r.upstreamProxy != nil && r.upstreamProxy.IsUpstream(req.Context(), upstreamNs) {
		repoName := filepath.Base(fullRepo)
		log.Printf("[UPSTREAM PROXY] Proxying tags list request: %s", fullRepo)
		upstreamResp, err := r.upstreamProxy.ProxyTagsList(req.Context(), upstreamNs, repoName)
		if err != nil {
			log.Printf("[UPSTREAM PROXY] Error: %v", err)
			http.Error(w, fmt.Sprintf(`{"errors":[{"code":"UPSTREAM_ERROR","message":"%s"}]}`, err.Error()), http.StatusBadGateway)
			return
		}
		if err := proxy.WriteProxyResponse(w, upstreamResp); err != nil {
			log.Printf("[UPSTREAM PROXY] Error writing response: %v", err)
		}
		return
	}

	// Normal local registry behavior
	limit := 0
	if nStr != "" {
		parsed, err := strconv.Atoi(nStr)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}

	tags, err := r.db.ListTags(req.Context(), fullRepo, limit, last)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if tags == nil {
		tags = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	if limit > 0 && len(tags) == limit {
		lastItem := tags[len(tags)-1]
		w.Header().Set("Link", fmt.Sprintf(`</v2/%s/tags/list?n=%d&last=%s>; rel="next"`, fullRepo, limit, lastItem))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"name": fullRepo,
		"tags": tags,
	})
}

// ----------------------------------------------------
// Manifests
// ----------------------------------------------------

func (r *Router) getManifest(w http.ResponseWriter, req *http.Request) {
	// Extract repository context from direct URL inspection
	fullRepo, quotaNs := getRepoContext(req)
	ref := chi.URLParam(req, "reference")

	// Check if this is an upstream registry request
	upstreamNs := quotaNs
	if r.upstreamProxy != nil && r.upstreamProxy.IsUpstream(req.Context(), upstreamNs) {
		repoName := filepath.Base(fullRepo)
		log.Printf("[UPSTREAM PROXY] Proxying manifest request: %s:%s", fullRepo, ref)
		upstreamResp, err := r.upstreamProxy.ProxyManifest(req.Context(), upstreamNs, repoName, ref, req.Method)
		if err != nil {
			log.Printf("[UPSTREAM PROXY] Error: %v", err)
			http.Error(w, fmt.Sprintf(`{"errors":[{"code":"UPSTREAM_ERROR","message":"%s"}]}`, err.Error()), http.StatusBadGateway)
			return
		}
		if err := proxy.WriteProxyResponse(w, upstreamResp); err != nil {
			log.Printf("[UPSTREAM PROXY] Error writing response: %v", err)
		}
		return
	}

	// Normal local registry behavior
	mediaType, digest, payload, err := r.db.GetManifest(req.Context(), fullRepo, ref)
	if err == db.ErrNotFound {
		http.Error(w, `{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Vulnerability gate: block pulls of images with blocked severities.
	if gateErr := r.checkVulnGate(req.Context(), digest); gateErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{{
				"code":    "DENIED",
				"message": gateErr.Error(),
			}},
		})
		return
	}

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	if req.Method == "GET" {
		w.Write(payload)
	}
}

func (r *Router) putManifest(w http.ResponseWriter, req *http.Request) {
	// Extract repository context from direct URL inspection
	fullRepo, quotaNs := getRepoContext(req)
	ref := chi.URLParam(req, "reference")

	log.Printf("[PUT_MANIFEST] Starting: fullRepo=%s namespace_context=%s ref=%s ContentLength=%d", fullRepo, quotaNs, ref, req.ContentLength)

	// Block overwrites of immutable tags
	if ref != "" && !strings.HasPrefix(ref, "sha256:") {
		immutable, err := r.db.IsTagImmutable(req.Context(), fullRepo, ref)
		if err == nil && immutable {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": []map[string]string{{
					"code":    "TAG_IMMUTABLE",
					"message": fmt.Sprintf("tag %s is immutable", ref),
				}},
			})
			return
		}
	}

	// Limit manifest size to 10MB
	maxManifestSize := int64(10 * 1024 * 1024)
	if req.ContentLength > maxManifestSize {
		errMsg := fmt.Sprintf("manifest exceeds maximum size of %d bytes", maxManifestSize)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Registry-Debug-Error", errMsg)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{
					"code":    "MANIFEST_TOO_LARGE",
					"message": errMsg,
				},
			},
		})
		return
	}

	mediaType := req.Header.Get("Content-Type")
	limitedReader := io.LimitReader(req.Body, maxManifestSize)
	payload, err := io.ReadAll(limitedReader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Verify it's valid JSON and compute canonical digest
	var manifest map[string]interface{}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{
					"code":    "MANIFEST_INVALID",
					"message": "invalid JSON manifest",
				},
			},
		})
		return
	}

	// Re-marshal in canonical form (sorted keys, no whitespace)
	canonical, err := json.Marshal(manifest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Compute digest using canonical JSON
	hasher := sha256.New()
	hasher.Write(canonical)
	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))

	// Enforce Quota (use namespace for quota checking)
	quota, err := r.db.CheckQuota(req.Context(), quotaNs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if quota != nil {
		// Calculate new quota
		if quota.UsedBytes+int64(len(canonical)) > quota.LimitBytes {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": []map[string]string{
					{
						"code":    "QUOTA_EXCEEDED",
						"message": fmt.Sprintf("namespace %s has exceeded its quota of %d bytes", quotaNs, quota.LimitBytes),
					},
				},
			})
			return
		}
	}

	// Store the manifest, artifact metadata, and quota update atomically.
	err = r.db.WithTx(req.Context(), func(ctx context.Context) error {
		if err := r.db.PutManifest(ctx, fullRepo, ref, mediaType, digest, canonical); err != nil {
			return err
		}
		if metadata := extractArtifactMetadata(canonical, mediaType, digest); metadata != nil {
			if err := r.db.SetArtifactMetadata(ctx, metadata); err != nil {
				log.Printf("[PUT_MANIFEST] Warning: failed to store artifact metadata: %v", err)
			} else {
				log.Printf("[PUT_MANIFEST] Stored artifact metadata: type=%s subject=%s", metadata.ArtifactType, metadata.SubjectDigest)
			}
		}
		if quota != nil {
			r.db.UpdateQuotaUsage(ctx, quotaNs, int64(len(canonical)))
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Location", fmt.Sprintf("/v2/%s/manifests/%s", fullRepo, digest))
	w.WriteHeader(http.StatusCreated)
	log.Printf("[PUT_MANIFEST] Success: fullRepo=%s ref=%s digest=%s", fullRepo, ref, digest)

	// Async: Execute embedded Automation Starlark scripts
	if r.starlark != nil {
		go func() {
			eventPayload := map[string]string{
				"namespace":  quotaNs,
				"repository": fullRepo,
				"reference":  ref,
				"digest":     digest,
			}
			// Load from a potential config path, hardcoded to local directory for MVP demonstration
			// In production, this would map to a ConfigMap or a /scripts volume mount.
			hookPath := "scripts/post_push.star"

			// We only trigger if the user actually configured the script file
			if _, hookErr := os.Stat(hookPath); hookErr == nil {
				if execErr := r.starlark.ExecuteEvent(hookPath, "push", eventPayload); execErr != nil {
					log.Printf("[Starlark Engine] Error executing on_push hook: %v", execErr)
				}
			}
		}()
	}

	// Async: Webhook Dispatcher + SSE broker
	eventData := map[string]string{
		"namespace":  quotaNs,
		"repository": fullRepo,
		"reference":  ref,
		"digest":     digest,
	}
	if r.webhook != nil {
		go r.webhook.Dispatch(context.Background(), "push", eventData)
	}
	if r.broker != nil {
		r.broker.Publish("push", eventData)
	}

	// Async: Trigger vulnerability scan via DarkScan
	if r.scanner != nil && r.scanner.IsEnabled() {
		go func() {
			registry := req.Host // e.g., "registry.afterdarksys.com"
			tag := ref           // Could be tag or digest

			ctx := req.Context()
			if err := r.scanner.ScanImage(ctx, registry, fullRepo, tag, digest, mediaType); err != nil {
				log.Printf("[SCANNER] Failed to submit scan for %s:%s - %v", fullRepo, tag, err)
			}
		}()
	}

	// Trigger async master-peer synchronization
	if r.syncManager != nil {
		r.syncManager.EnqueuePush(quotaNs, fullRepo, ref, digest)
	}
}

// ----------------------------------------------------
// Blobs
// ----------------------------------------------------

func (r *Router) getBlob(w http.ResponseWriter, req *http.Request) {
	// Extract repository context from direct URL inspection
	fullRepo, quotaNs := getRepoContext(req)
	digest := chi.URLParam(req, "digest")

	// Check if this is an upstream registry request
	upstreamNs := quotaNs
	if r.upstreamProxy != nil && r.upstreamProxy.IsUpstream(req.Context(), upstreamNs) {
		repoName := filepath.Base(fullRepo)
		log.Printf("[UPSTREAM PROXY] Proxying blob request: %s %s", fullRepo, digest)
		upstreamResp, err := r.upstreamProxy.ProxyBlob(req.Context(), upstreamNs, repoName, digest, req.Method)
		if err != nil {
			log.Printf("[UPSTREAM PROXY] Error: %v", err)
			http.Error(w, fmt.Sprintf(`{"errors":[{"code":"UPSTREAM_ERROR","message":"%s"}]}`, err.Error()), http.StatusBadGateway)
			return
		}
		if err := proxy.WriteProxyResponse(w, upstreamResp); err != nil {
			log.Printf("[UPSTREAM PROXY] Error writing response: %v", err)
		}
		return
	}

	// Normal local registry behavior
	size, err := r.db.GetBlobSize(req.Context(), digest)
	if err == db.ErrNotFound {
		http.Error(w, `{"errors":[{"code":"BLOB_UNKNOWN","message":"blob unknown"}]}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Docker-Content-Digest", digest)

	if req.Method == "GET" {
		path := getPath(fullRepo, digest)
		// Handle Range header for partial blob downloads
		if rangeHeader := req.Header.Get("Range"); rangeHeader != "" {
			// Expected format: bytes=start-
			var start int64
			if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start); err == nil && start >= 0 {
				reader, err := r.storage.Reader(req.Context(), path, start)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				defer reader.Close()
				w.WriteHeader(http.StatusPartialContent)
				io.Copy(w, reader)
				return
			}
		}
		// No Range header – serve full blob
		reader, err := r.storage.Reader(req.Context(), path, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer reader.Close()
		io.Copy(w, reader)
	}
}

func (r *Router) headBlob(w http.ResponseWriter, req *http.Request) {
	r.getBlob(w, req)
}

// ----------------------------------------------------
// Uploads (Monolithic or chunked)
// ----------------------------------------------------

func (r *Router) startUpload(w http.ResponseWriter, req *http.Request) {
	// Extract repository context from direct URL inspection
	fullRepo, quotaNs := getRepoContext(req)

	// Intercept Cross-Repository Blob Mounts
	// Format: POST /v2/<name>/blobs/uploads/?mount=<digest>&from=<repository name>
	mountDigest := req.URL.Query().Get("mount")
	fromRepo := req.URL.Query().Get("from")

	if mountDigest != "" && fromRepo != "" {
		log.Printf("[START_UPLOAD] Cross-repo mount requested: fullRepo=%s fromRepo=%s digest=%s", fullRepo, fromRepo, mountDigest)
		fromPath := getPath(fromRepo, mountDigest)
		size, err := r.storage.Stat(req.Context(), fromPath)

		// If blob exists in the 'from' repository, we can mount/copy it
		if err == nil {
			// 1. Quota check against the target namespace
			quota, err := r.db.CheckQuota(req.Context(), quotaNs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if quota != nil && quota.UsedBytes+size > quota.LimitBytes {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errors": []map[string]string{{"code": "QUOTA_EXCEEDED", "message": "quota exceeded"}},
				})
				return
			}

			// 2. Perform copy
			reader, err := r.storage.Reader(req.Context(), fromPath, 0)
			if err == nil {
				targetPath := getPath(fullRepo, mountDigest)
				writer, err := r.storage.Writer(req.Context(), targetPath)
				if err == nil {
					_, copyErr := io.Copy(writer, reader)
					writer.Close()
					reader.Close()

					if copyErr == nil {
						// 3. Mount successful: Update database and return 201 Created
						r.db.PutBlob(req.Context(), mountDigest, size, "application/octet-stream")
						if quota != nil {
							r.db.UpdateQuotaUsage(req.Context(), quotaNs, size)
						}

						w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", fullRepo, mountDigest))
						w.Header().Set("Docker-Content-Digest", mountDigest)
						w.WriteHeader(http.StatusCreated)
						return
					}
					// Cleanup partial copies if io.Copy fails
					r.storage.Delete(req.Context(), targetPath)
				} else {
					reader.Close()
				}
			}
		} else {
			log.Printf("[START_UPLOAD] Cross-repo mount failed: blob not found in fromRepo")
		}
	}

	uploadUUID := uuid.New().String()

	log.Printf("[START_UPLOAD] fullRepo=%s namespace_context=%s uuid=%s", fullRepo, quotaNs, uploadUUID)

	// Create an empty temporary file to track the upload state
	tempPath := getPath(fullRepo, "uploads/"+uploadUUID)
	writer, err := r.storage.Writer(req.Context(), tempPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Close()

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", fullRepo, uploadUUID))
	w.Header().Set("Range", "0-0")
	w.Header().Set("Docker-Upload-UUID", uploadUUID)
	w.WriteHeader(http.StatusAccepted)
}

// getUploadProgress returns the current upload progress for resumable uploads
// Implements GET /v2/{name}/blobs/uploads/{uuid}
// Returns 204 No Content with Range header indicating bytes received
func (r *Router) getUploadProgress(w http.ResponseWriter, req *http.Request) {
	fullRepo, _ := getRepoContext(req)
	uuid := chi.URLParam(req, "uuid")

	tempPath := getPath(fullRepo, "uploads/"+uuid)
	size, err := r.storage.Stat(req.Context(), tempPath)
	if err != nil {
		if err == storage.ErrNotFound {
			http.Error(w, "upload not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculate range end (0-based, so size-1, but if size is 0, report 0-0)
	rangeEnd := size - 1
	if rangeEnd < 0 {
		rangeEnd = 0
		size = 0
	}

	w.Header().Set("Range", fmt.Sprintf("0-%d", rangeEnd))
	w.Header().Set("Docker-Upload-UUID", uuid)
	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", fullRepo, uuid))
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) patchUpload(w http.ResponseWriter, req *http.Request) {
	// Normally appends data to a temp file
	// Extract repository context from direct URL inspection
	fullRepo, _ := getRepoContext(req)
	uuid := chi.URLParam(req, "uuid")

	// Serialize concurrent PATCH requests for the same UUID to prevent
	// data corruption from simultaneous appends to the same temp file.
	unlock := r.lockUpload(uuid)
	defer unlock()

	// Limit upload size per chunk
	maxUploadSize := int64(10 * 1024 * 1024 * 1024) // 10GB
	if req.ContentLength > maxUploadSize {
		errMsg := fmt.Sprintf("upload chunk exceeds maximum size of %d bytes", maxUploadSize)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Registry-Debug-Error", errMsg)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{
					"code":    "UPLOAD_TOO_LARGE",
					"message": errMsg,
				},
			},
		})
		return
	}

	tempPath := getPath(fullRepo, "uploads/"+uuid)
	appender, err := r.storage.Appender(req.Context(), tempPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Use limited reader to enforce size limit at read time
	limitedReader := io.LimitReader(req.Body, maxUploadSize)
	_, err = io.Copy(appender, limitedReader)
	
	// Explicitly close the appender to flush any buffered local writers
	// before we stat the file size!
	closeErr := appender.Close()
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if closeErr != nil {
		http.Error(w, closeErr.Error(), http.StatusInternalServerError)
		return
	}

	size, err := r.storage.Stat(req.Context(), tempPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rangeEnd := size - 1
	if rangeEnd < 0 {
		rangeEnd = 0
	}

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", fullRepo, uuid))
	w.Header().Set("Range", fmt.Sprintf("0-%d", rangeEnd))
	w.Header().Set("Docker-Upload-UUID", uuid)
	w.WriteHeader(http.StatusAccepted)
}

func (r *Router) putUpload(w http.ResponseWriter, req *http.Request) {
	// Extract repository context from direct URL inspection
	fullRepo, quotaNs := getRepoContext(req)
	uuid := chi.URLParam(req, "uuid")
	digest := req.URL.Query().Get("digest")

	log.Printf("[PUT_UPLOAD] fullRepo=%s namespace_context=%s uuid=%s digest=%s", fullRepo, quotaNs, uuid, digest)

	if digest == "" {
		http.Error(w, "digest query param required", http.StatusBadRequest)
		return
	}

	tempPath := getPath(fullRepo, "uploads/"+uuid)
	finalPath := getPath(fullRepo, digest)

	// Idempotency check: Does this blob already exist?
	exists, err := r.db.BlobExists(req.Context(), digest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		// Clean up partial upload just in case
		r.storage.Delete(req.Context(), tempPath)
		w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", fullRepo, digest))
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusCreated)
		return
	}

	var size int64
	var actualDigest string

	// Check if this is a monolithic upload (temp file doesn't exist yet or is empty)
	// or chunked upload (temp file was created and populated by prior PATCH requests)
	tempFileExists := true
	if size, err := r.storage.Stat(req.Context(), tempPath); err != nil {
		if err == storage.ErrNotFound {
			tempFileExists = false
		}
	} else if size == 0 {
		// startUpload always creates a 0-byte file. If it's still 0 bytes,
		// no PATCH was performed and we should use the monolithic upload optimization.
		tempFileExists = false
	}

	// If there's a body, append it (monolithic upload or final chunk)
	if req.ContentLength > 0 || req.Body != http.NoBody {
		if tempFileExists {
			// Chunked upload: append final chunk to existing temp file
			appender, err := r.storage.Appender(req.Context(), tempPath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, err = io.Copy(appender, req.Body)
			
			// Always check close error, as this flushes the buffer to disk
			closeErr := appender.Close()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if closeErr != nil {
				http.Error(w, closeErr.Error(), http.StatusInternalServerError)
				return
			}

			// For chunked uploads, we must read the entire accumulated file to compute hash
			reader, err := r.storage.Reader(req.Context(), tempPath, 0)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			hasher := sha256.New()
			size, err = io.Copy(hasher, reader)
			reader.Close()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			actualDigest = "sha256:" + hex.EncodeToString(hasher.Sum(nil))
		} else {
			// Monolithic upload: CRITICAL FIX - compute digest DURING upload
			// This eliminates the blocking re-read for large layers (common case)
			appender, err := r.storage.Appender(req.Context(), tempPath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			hasher := sha256.New()
			multiWriter := io.MultiWriter(appender, hasher)
			size, err = io.Copy(multiWriter, req.Body)
			
			// Always check close error, as this flushes the buffer to disk
			closeErr := appender.Close()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if closeErr != nil {
				http.Error(w, closeErr.Error(), http.StatusInternalServerError)
				return
			}
			actualDigest = "sha256:" + hex.EncodeToString(hasher.Sum(nil))
		}
	} else {
		// No body - compute digest from existing temp file (chunked upload finalization)
		reader, err := r.storage.Reader(req.Context(), tempPath, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		hasher := sha256.New()
		size, err = io.Copy(hasher, reader)
		reader.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		actualDigest = "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	}

	// Verify digest matches expected
	if actualDigest != digest {
		// Digest mismatch - delete uploaded file and return error
		r.storage.Delete(req.Context(), tempPath)
		errMsg := fmt.Sprintf("digest mismatch: expected %s, got %s", digest, actualDigest)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Registry-Debug-Error", errMsg)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{
					"code":    "DIGEST_INVALID",
					"message": errMsg,
				},
			},
		})
		return
	}

	// Enforce Quota (use namespace for quota checking)
	quota, err := r.db.CheckQuota(req.Context(), quotaNs)
	if err != nil {
		r.storage.Delete(req.Context(), tempPath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if quota != nil {
		if quota.UsedBytes+size > quota.LimitBytes {
			r.storage.Delete(req.Context(), tempPath)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": []map[string]string{
					{
						"code":    "QUOTA_EXCEEDED",
						"message": fmt.Sprintf("namespace %s has exceeded its quota of %d bytes", quotaNs, quota.LimitBytes),
					},
				},
			})
			return
		}
	}

	// Use singleflight to deduplicate concurrent finalizations of the same
	// digest. Only the first caller performs the move+DB write; others share
	// the result.
	_, err, _ = r.blobGroup.Do(digest, func() (interface{}, error) {
		// 1. Move the verified temp file to final location.
		if moveErr := r.storage.Move(req.Context(), tempPath, finalPath); moveErr != nil {
			return nil, moveErr
		}
		// 2. Record blob and update quota atomically.
		txErr := r.db.WithTx(req.Context(), func(ctx context.Context) error {
			if err := r.db.PutBlob(ctx, digest, size, "application/octet-stream"); err != nil {
				return err
			}
			if quota != nil {
				r.db.UpdateQuotaUsage(ctx, quotaNs, size)
			}
			return nil
		})
		if txErr != nil {
			r.storage.Delete(req.Context(), finalPath)
			return nil, txErr
		}
		return nil, nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Upload is finalized; remove the per-UUID mutex so it can be GC'd.
	r.uploadMu.Delete(uuid)

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", fullRepo, digest))
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusCreated)
}

func (r *Router) deleteManifest(w http.ResponseWriter, req *http.Request) {
	// Extract repository context from direct URL inspection
	fullRepo, quotaNs := getRepoContext(req)
	ref := chi.URLParam(req, "reference")

	// 1. Fetch manifest to find its digest and payload size before deleting
	_, digest, payload, err := r.db.GetManifest(req.Context(), fullRepo, ref)
	if err == db.ErrNotFound {
		http.Error(w, `{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Perform deletion from database
	if err := r.db.DeleteManifest(req.Context(), fullRepo, ref); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Update Quota Usage (Reclaim bytes)
	// Note: We only delete the manifest record, the underlying blob might optionally be deleted or garbage collected later
	r.db.UpdateQuotaUsage(req.Context(), quotaNs, -int64(len(payload)))

	// 4. Trigger Webhook Event + SSE broker
	deleteData := map[string]string{
		"namespace":  quotaNs,
		"repository": fullRepo,
		"reference":  ref,
		"digest":     digest,
	}
	if r.webhook != nil {
		go r.webhook.Dispatch(context.Background(), "delete", deleteData)
	}
	if r.broker != nil {
		r.broker.Publish("delete", deleteData)
	}

	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.WriteHeader(http.StatusAccepted) // Docker Registry API expects 202 Accepted for delete
}
