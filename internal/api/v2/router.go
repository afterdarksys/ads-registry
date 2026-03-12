package v2

import (
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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/automation"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/policy"
	"github.com/ryan/ads-registry/internal/proxy"
	"github.com/ryan/ads-registry/internal/storage"
	"github.com/ryan/ads-registry/internal/upstreams"
)

type Router struct {
	db            db.Store
	storage       storage.Provider
	authMid       *auth.Middleware
	tokenTs       *auth.TokenService
	enforcer      *policy.Enforcer
	starlark      *automation.Engine
	upstreamProxy *proxy.UpstreamProxy
}

func NewRouter(dbStore db.Store, storageProvider storage.Provider, ts *auth.TokenService, enf *policy.Enforcer, star *automation.Engine, upstreamMgr *upstreams.Manager) *Router {
	var upstreamProxy *proxy.UpstreamProxy
	if upstreamMgr != nil {
		upstreamProxy = proxy.NewUpstreamProxy(upstreamMgr)
	}

	return &Router{
		db:            dbStore,
		storage:       storageProvider,
		tokenTs:       ts,
		authMid:       auth.NewMiddleware(ts),
		enforcer:      enf,
		starlark:      star,
		upstreamProxy: upstreamProxy,
	}
}

func (r *Router) Register(mux chi.Router) {

	// The Auth Endpoint
	authHandler := auth.NewHandler(r.tokenTs, r.db)
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

		api.Route("/{namespace}/{repo}", func(repoCtx chi.Router) {

			// 1. First, check Auth JWT Bearer Token scopes
			// NOTE: Middleware must be applied HERE, not at /v2 level,
			// so that namespace/repo URL params are available for authorization
			repoCtx.Use(r.authMid.Protect)

			// 2. Second, invoke the CEL engine to verify custom enterprise admission rules
			// DISABLED for initial testing - uncomment to enable policy enforcement
			// if r.enforcer != nil {
			// 	repoCtx.Use(r.enforcer.Protect)
			// }

			// Tags
			repoCtx.Get("/tags/list", r.getTags)

			// Manifests
			repoCtx.Get("/manifests/{reference}", r.getManifest)
			repoCtx.Head("/manifests/{reference}", r.getManifest)
			repoCtx.Put("/manifests/{reference}", r.putManifest)

			// Blobs
			repoCtx.Get("/blobs/{digest}", r.getBlob)
			repoCtx.Head("/blobs/{digest}", r.headBlob)

			// Uploads
			repoCtx.Post("/blobs/uploads/", r.startUpload)
			repoCtx.Patch("/blobs/uploads/{uuid}", r.patchUpload)
			repoCtx.Put("/blobs/uploads/{uuid}", r.putUpload)
		})
	})
}

func (r *Router) baseCheck(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")

	// If no auth provided, tell Docker where to get a token
	if req.Header.Get("Authorization") == "" {
		w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/auth/token",service="registry"`, req.Host))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getPath(ns, repo, digest string) string {
	return filepath.Join(ns, repo, digest)
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
	ns := chi.URLParam(req, "namespace")
	repo := chi.URLParam(req, "repo")
	nStr := req.URL.Query().Get("n")
	last := req.URL.Query().Get("last")

	// Check if this is an upstream registry request
	if r.upstreamProxy != nil && r.upstreamProxy.IsUpstream(req.Context(), ns) {
		log.Printf("[UPSTREAM PROXY] Proxying tags list request: %s/%s", ns, repo)
		upstreamResp, err := r.upstreamProxy.ProxyTagsList(req.Context(), ns, repo)
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

	repoPath := filepath.Join(ns, repo)
	tags, err := r.db.ListTags(req.Context(), repoPath, limit, last)
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
		w.Header().Set("Link", fmt.Sprintf(`</v2/%s/%s/tags/list?n=%d&last=%s>; rel="next"`, ns, repo, limit, lastItem))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"name": repoPath,
		"tags": tags,
	})
}

// ----------------------------------------------------
// Manifests
// ----------------------------------------------------

func (r *Router) getManifest(w http.ResponseWriter, req *http.Request) {
	ns := chi.URLParam(req, "namespace")
	repo := chi.URLParam(req, "repo")
	ref := chi.URLParam(req, "reference")

	// Check if this is an upstream registry request
	if r.upstreamProxy != nil && r.upstreamProxy.IsUpstream(req.Context(), ns) {
		log.Printf("[UPSTREAM PROXY] Proxying manifest request: %s/%s:%s", ns, repo, ref)
		upstreamResp, err := r.upstreamProxy.ProxyManifest(req.Context(), ns, repo, ref, req.Method)
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
	mediaType, digest, payload, err := r.db.GetManifest(req.Context(), filepath.Join(ns, repo), ref)
	if err == db.ErrNotFound {
		http.Error(w, `{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	ns := chi.URLParam(req, "namespace")
	repo := chi.URLParam(req, "repo")
	ref := chi.URLParam(req, "reference")

	log.Printf("[PUT_MANIFEST] Starting: ns=%s repo=%s ref=%s ContentLength=%d", ns, repo, ref, req.ContentLength)

	// Limit manifest size to 10MB
	maxManifestSize := int64(10 * 1024 * 1024)
	if req.ContentLength > maxManifestSize {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{
					"code":    "MANIFEST_TOO_LARGE",
					"message": fmt.Sprintf("manifest exceeds maximum size of %d bytes", maxManifestSize),
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

	// Enforce Quota
	quota, err := r.db.CheckQuota(req.Context(), ns)
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
						"message": fmt.Sprintf("namespace %s has exceeded its quota of %d bytes", ns, quota.LimitBytes),
					},
				},
			})
			return
		}
	}

	// Store the canonical form
	err = r.db.PutManifest(req.Context(), filepath.Join(ns, repo), ref, mediaType, digest, canonical)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update Quota Usage
	if quota != nil {
		r.db.UpdateQuotaUsage(req.Context(), ns, int64(len(canonical)))
	}

	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Location", "/v2/"+ns+"/"+repo+"/manifests/"+digest)
	w.WriteHeader(http.StatusCreated)
	log.Printf("[PUT_MANIFEST] Success: ns=%s repo=%s ref=%s digest=%s", ns, repo, ref, digest)

	// Async: Execute embedded Automation Starlark scripts
	if r.starlark != nil {
		go func() {
			eventPayload := map[string]string{
				"namespace":  ns,
				"repository": repo,
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
}

// ----------------------------------------------------
// Blobs
// ----------------------------------------------------

func (r *Router) getBlob(w http.ResponseWriter, req *http.Request) {
	ns := chi.URLParam(req, "namespace")
	repo := chi.URLParam(req, "repo")
	digest := chi.URLParam(req, "digest")

	// Check if this is an upstream registry request
	if r.upstreamProxy != nil && r.upstreamProxy.IsUpstream(req.Context(), ns) {
		log.Printf("[UPSTREAM PROXY] Proxying blob request: %s/%s %s", ns, repo, digest)
		upstreamResp, err := r.upstreamProxy.ProxyBlob(req.Context(), ns, repo, digest, req.Method)
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
		path := getPath(ns, repo, digest)
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
	ns := chi.URLParam(req, "namespace")
	repo := chi.URLParam(req, "repo")
	uploadUUID := uuid.New().String()

	// Create an empty temporary file to track the upload state
	tempPath := filepath.Join(ns, repo, "uploads", uploadUUID)
	writer, err := r.storage.Writer(req.Context(), tempPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Close()

	w.Header().Set("Location", "/v2/"+ns+"/"+repo+"/blobs/uploads/"+uploadUUID)
	w.Header().Set("Range", "0-0")
	w.Header().Set("Docker-Upload-UUID", uploadUUID)
	w.WriteHeader(http.StatusAccepted)
}

func (r *Router) patchUpload(w http.ResponseWriter, req *http.Request) {
	// Normally appends data to a temp file
	ns := chi.URLParam(req, "namespace")
	repo := chi.URLParam(req, "repo")
	uuid := chi.URLParam(req, "uuid")

	// Limit upload size per chunk
	maxUploadSize := int64(10 * 1024 * 1024 * 1024) // 10GB
	if req.ContentLength > maxUploadSize {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{
					"code":    "UPLOAD_TOO_LARGE",
					"message": fmt.Sprintf("upload chunk exceeds maximum size of %d bytes", maxUploadSize),
				},
			},
		})
		return
	}

	tempPath := filepath.Join(ns, repo, "uploads", uuid)
	appender, err := r.storage.Appender(req.Context(), tempPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer appender.Close()

	// Use limited reader to enforce size limit at read time
	limitedReader := io.LimitReader(req.Body, maxUploadSize)
	_, err = io.Copy(appender, limitedReader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// We'd ideally track the size properly to return the correct Range
	w.Header().Set("Location", "/v2/"+ns+"/"+repo+"/blobs/uploads/"+uuid)
	w.Header().Set("Range", "0-0") // Assuming 0-0 for MVP as docker clients often ignore it if Location is intact
	w.Header().Set("Docker-Upload-UUID", uuid)
	w.WriteHeader(http.StatusAccepted)
}

func (r *Router) putUpload(w http.ResponseWriter, req *http.Request) {
	ns := chi.URLParam(req, "namespace")
	repo := chi.URLParam(req, "repo")
	uuid := chi.URLParam(req, "uuid")
	digest := req.URL.Query().Get("digest")

	if digest == "" {
		http.Error(w, "digest query param required", http.StatusBadRequest)
		return
	}

	tempPath := filepath.Join(ns, repo, "uploads", uuid)
	finalPath := getPath(ns, repo, digest)

	// Idempotency check: Does this blob already exist?
	exists, err := r.db.BlobExists(req.Context(), digest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		// Clean up partial upload just in case
		r.storage.Delete(req.Context(), tempPath)
		w.Header().Set("Location", "/v2/"+ns+"/"+repo+"/blobs/"+digest)
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusCreated)
		return
	}

	var size int64
	var errCopy error

	// If there's a body, it's a monolithic upload or the final chunk
	// Try appended reading
	if req.ContentLength > 0 || req.Body != http.NoBody {
		appender, err := r.storage.Appender(req.Context(), tempPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, errCopy = io.Copy(appender, req.Body)
		appender.Close()
		if errCopy != nil {
			http.Error(w, errCopy.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Verify digest by computing hash of uploaded content
	reader, err := r.storage.Reader(req.Context(), tempPath, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hasher := sha256.New()
	size, errCopy = io.Copy(hasher, reader)
	reader.Close()
	if errCopy != nil {
		http.Error(w, errCopy.Error(), http.StatusInternalServerError)
		return
	}

	actualDigest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if actualDigest != digest {
		// Digest mismatch - delete uploaded file and return error
		r.storage.Delete(req.Context(), tempPath)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{
					"code":    "DIGEST_INVALID",
					"message": fmt.Sprintf("digest mismatch: expected %s, got %s", digest, actualDigest),
				},
			},
		})
		return
	}

	// Enforce Quota
	quota, err := r.db.CheckQuota(req.Context(), ns)
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
						"message": fmt.Sprintf("namespace %s has exceeded its quota of %d bytes", ns, quota.LimitBytes),
					},
				},
			})
			return
		}
	}

	// 1. Move the verified temp file to final location
	err = r.storage.Move(req.Context(), tempPath, finalPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Record in database
	err = r.db.PutBlob(req.Context(), digest, size, "application/octet-stream")
	if err != nil {
		// Rollback file move on DB failure
		r.storage.Delete(req.Context(), finalPath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update Quota Usage
	if quota != nil {
		r.db.UpdateQuotaUsage(req.Context(), ns, size)
	}

	w.Header().Set("Location", "/v2/"+ns+"/"+repo+"/blobs/"+digest)
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusCreated)
}
