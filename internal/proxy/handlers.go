package proxy

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/metrics"
)

// Handler handles HTTP requests for pull-through cache
type Handler struct {
	proxy *RegistryProxy
}

// NewHandler creates a new proxy handler
func NewHandler(proxy *RegistryProxy) *Handler {
	return &Handler{
		proxy: proxy,
	}
}

// RegisterRoutes registers proxy routes with chi router
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Proxy manifest endpoints
	r.Get("/v2/proxy/{upstream}/{repo:.+}/manifests/{reference}", h.ProxyManifestHandler)

	// Proxy blob endpoints
	r.Get("/v2/proxy/{upstream}/{repo:.+}/blobs/{digest}", h.ProxyBlobHandler)

	// Proxy catalog (list upstream repositories)
	r.Get("/v2/proxy/{upstream}/_catalog", h.ProxyCatalogHandler)

	// Proxy tags list
	r.Get("/v2/proxy/{upstream}/{repo:.+}/tags/list", h.ProxyTagsHandler)
}

// ProxyManifestHandler handles manifest pull requests with caching
// GET /v2/proxy/{upstream}/{repo}/manifests/{reference}
//
// Examples:
//   GET /v2/proxy/dockerhub/library/ubuntu/manifests/latest
//   GET /v2/proxy/gcr/google-samples/hello-app/manifests/1.0
func (h *Handler) ProxyManifestHandler(w http.ResponseWriter, r *http.Request) {
	upstream := chi.URLParam(r, "upstream")
	repo := chi.URLParam(r, "repo")
	reference := chi.URLParam(r, "reference")

	log.Printf("[ProxyHandler] Manifest request: %s/%s:%s", upstream, repo, reference)

	// Check if manifest is cached locally first
	localRepo := "proxy/" + upstream + "/" + repo
	if mediaType, digest, payload, err := h.proxy.db.GetManifest(r.Context(), localRepo, reference); err == nil {
		log.Printf("[ProxyHandler] Manifest cache HIT: %s:%s", localRepo, reference)
		metrics.ProxyCacheHits.WithLabelValues(upstream).Inc()
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		w.Write(payload)
		return
	}

	log.Printf("[ProxyHandler] Manifest cache MISS: %s:%s", localRepo, reference)
	metrics.ProxyCacheMisses.WithLabelValues(upstream).Inc()

	// Fetch from upstream and cache (track duration)
	startTime := time.Now()
	mediaType, digest, payload, err := h.proxy.ProxyManifest(r.Context(), upstream, repo, reference)
	duration := time.Since(startTime).Seconds()
	metrics.UpstreamFetchDuration.WithLabelValues(upstream, "manifest").Observe(duration)

	if err != nil {
		log.Printf("[ProxyHandler] Failed to proxy manifest: %v", err)
		http.Error(w, "failed to fetch manifest from upstream", http.StatusBadGateway)
		return
	}

	// Return manifest to client
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

// ProxyBlobHandler handles blob pull requests with caching
// GET /v2/proxy/{upstream}/{repo}/blobs/{digest}
//
// Examples:
//   GET /v2/proxy/dockerhub/library/ubuntu/blobs/sha256:abc123...
//   GET /v2/proxy/gcr/google-samples/hello-app/blobs/sha256:def456...
func (h *Handler) ProxyBlobHandler(w http.ResponseWriter, r *http.Request) {
	upstream := chi.URLParam(r, "upstream")
	repo := chi.URLParam(r, "repo")
	digest := chi.URLParam(r, "digest")

	log.Printf("[ProxyHandler] Blob request: %s/%s@%s", upstream, repo, digest)

	// Fetch blob (will check cache first)
	startTime := time.Now()
	reader, size, cacheHit, err := h.proxy.ProxyBlob(r.Context(), upstream, repo, digest)
	if err != nil {
		log.Printf("[ProxyHandler] Failed to proxy blob: %v", err)
		http.Error(w, "failed to fetch blob from upstream", http.StatusBadGateway)
		return
	}
	defer reader.Close()

	// Track cache metrics
	if cacheHit {
		metrics.ProxyCacheHits.WithLabelValues(upstream).Inc()
	} else {
		metrics.ProxyCacheMisses.WithLabelValues(upstream).Inc()
		duration := time.Since(startTime).Seconds()
		metrics.UpstreamFetchDuration.WithLabelValues(upstream, "blob").Observe(duration)
	}

	// Stream blob to client
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, reader); err != nil {
		log.Printf("[ProxyHandler] Failed to stream blob: %v", err)
	}
}

// ProxyCatalogHandler lists repositories from upstream registry
// GET /v2/proxy/{upstream}/_catalog
func (h *Handler) ProxyCatalogHandler(w http.ResponseWriter, r *http.Request) {
	upstream := chi.URLParam(r, "upstream")

	log.Printf("[ProxyHandler] Catalog request for upstream: %s", upstream)

	// This would fetch the catalog from the upstream registry
	// For now, return empty catalog
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"repositories":[]}`))
}

// ProxyTagsHandler lists tags for a repository from upstream
// GET /v2/proxy/{upstream}/{repo}/tags/list
func (h *Handler) ProxyTagsHandler(w http.ResponseWriter, r *http.Request) {
	upstream := chi.URLParam(r, "upstream")
	repo := chi.URLParam(r, "repo")

	log.Printf("[ProxyHandler] Tags request: %s/%s", upstream, repo)

	// This would fetch tags from the upstream registry
	// For now, return empty tags list
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"name":"` + repo + `","tags":[]}`))
}

// AutoDetectProxyRequest checks if a request should be proxied
// This middleware can be added to automatically detect proxy requests
// DISABLED: Not currently used, and has compilation issues with chi.NewRouteContext().WithContext()
/*
func (h *Handler) AutoDetectProxyRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request path contains a known upstream prefix
		path := r.URL.Path

		// Examples:
		// /v2/gcr.io/project/image/manifests/tag → proxy to GCR
		// /v2/docker.io/library/ubuntu/manifests/latest → proxy to Docker Hub

		for _, prefix := range []string{"gcr.io/", "ghcr.io/", "quay.io/", "docker.io/"} {
			if strings.Contains(path, prefix) {
				// Extract upstream and rewrite path
				upstream := h.proxy.ResolveUpstream(path)
				repo := StripUpstreamPrefix(strings.TrimPrefix(path, "/v2/"))

				// Inject upstream parameter
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("upstream", upstream)
				rctx.URLParams.Add("repo", repo)

				log.Printf("[ProxyHandler] Auto-detected proxy request: %s → upstream=%s, repo=%s",
					path, upstream, repo)

				// Continue with modified context
				r = r.WithContext(chi.NewRouteContext().WithContext(r.Context()))
				next.ServeHTTP(w, r)
				return
			}
		}

		// Not a proxy request, continue normally
		next.ServeHTTP(w, r)
	})
}
*/
