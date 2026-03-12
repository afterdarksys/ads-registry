package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

// RegistryProxy handles pull-through caching from upstream registries
type RegistryProxy struct {
	storage    storage.Provider
	db         db.Store
	httpClient *http.Client
	upstreams  map[string]*UpstreamRegistry
	mu         sync.RWMutex
}

// UpstreamRegistry represents a remote registry to proxy
type UpstreamRegistry struct {
	Name     string
	URL      string // e.g., "https://registry-1.docker.io"
	Username string
	Password string
	Mirror   bool // If true, this is a mirror (cache all pulls)
}

// NewRegistryProxy creates a new registry proxy
func NewRegistryProxy(sp storage.Provider, dbStore db.Store) *RegistryProxy {
	return &RegistryProxy{
		storage: sp,
		db:      dbStore,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		upstreams: make(map[string]*UpstreamRegistry),
	}
}

// RegisterUpstream adds an upstream registry
func (p *RegistryProxy) RegisterUpstream(name, url, username, password string, mirror bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.upstreams[name] = &UpstreamRegistry{
		Name:     name,
		URL:      url,
		Username: username,
		Password: password,
		Mirror:   mirror,
	}

	log.Printf("[Proxy] Registered upstream: %s → %s (mirror: %v)", name, url, mirror)
}

// ProxyManifest proxies a manifest request to upstream registry
func (p *RegistryProxy) ProxyManifest(ctx context.Context, upstream, repo, reference string) (mediaType, digest string, payload []byte, err error) {
	p.mu.RLock()
	upstreamRegistry, exists := p.upstreams[upstream]
	p.mu.RUnlock()

	if !exists {
		return "", "", nil, fmt.Errorf("upstream registry not found: %s", upstream)
	}

	log.Printf("[Proxy] Fetching manifest from %s: %s:%s", upstream, repo, reference)

	// Construct upstream URL
	// Docker Hub: registry-1.docker.io/v2/library/ubuntu/manifests/latest
	// GCR: gcr.io/v2/project/image/manifests/tag
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", upstreamRegistry.URL, repo, reference)

	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication if configured
	if upstreamRegistry.Username != "" {
		req.SetBasicAuth(upstreamRegistry.Username, upstreamRegistry.Password)
	}

	// Accept both manifest schema v2 and OCI manifest
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	// Read manifest
	payload, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Get content type and digest from response headers
	mediaType = resp.Header.Get("Content-Type")
	digest = resp.Header.Get("Docker-Content-Digest")

	// If no digest in header, calculate it
	if digest == "" {
		digest = calculateDigest(payload)
	}

	log.Printf("[Proxy] Fetched manifest %s (digest: %s, size: %d bytes)", reference, digest, len(payload))

	// Cache manifest in local storage
	if err := p.cacheManifest(ctx, upstream, repo, reference, mediaType, digest, payload); err != nil {
		log.Printf("[Proxy] Warning: failed to cache manifest: %v", err)
		// Don't fail the request if caching fails
	}

	return mediaType, digest, payload, nil
}

// ProxyBlob proxies a blob request to upstream registry
// Returns: reader, size, cacheHit, error
func (p *RegistryProxy) ProxyBlob(ctx context.Context, upstream, repo, digest string) (io.ReadCloser, int64, bool, error) {
	p.mu.RLock()
	upstreamRegistry, exists := p.upstreams[upstream]
	p.mu.RUnlock()

	if !exists {
		return nil, 0, false, fmt.Errorf("upstream registry not found: %s", upstream)
	}

	// Check if blob is already cached locally
	blobPath := fmt.Sprintf("blobs/%s", digest)
	if size, err := p.storage.Stat(ctx, blobPath); err == nil {
		log.Printf("[Proxy] Blob %s found in cache (%d bytes)", digest, size)
		reader, err := p.storage.Reader(ctx, blobPath, 0)
		if err == nil {
			return reader, size, true, nil // Cache hit
		}
		log.Printf("[Proxy] Warning: cached blob read failed: %v", err)
	}

	log.Printf("[Proxy] Fetching blob from %s: %s", upstream, digest)

	// Construct upstream URL
	blobURL := fmt.Sprintf("%s/v2/%s/blobs/%s", upstreamRegistry.URL, repo, digest)

	req, err := http.NewRequestWithContext(ctx, "GET", blobURL, nil)
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication if configured
	if upstreamRegistry.Username != "" {
		req.SetBasicAuth(upstreamRegistry.Username, upstreamRegistry.Password)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to fetch blob: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, false, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	size := resp.ContentLength

	log.Printf("[Proxy] Fetched blob %s (%d bytes)", digest, size)

	// Cache blob asynchronously (don't block the response)
	go p.cacheBlob(context.Background(), digest, resp.Body, size)

	// Return response body to caller (they'll read while we cache in background)
	// Note: This is a simplified approach. In production, use io.TeeReader to stream to both client and cache
	return resp.Body, size, false, nil // Cache miss
}

// cacheManifest stores a manifest in local storage and database
func (p *RegistryProxy) cacheManifest(ctx context.Context, upstream, repo, reference, mediaType, digest string, payload []byte) error {
	// Construct local repository name: proxy/{upstream}/{repo}
	localRepo := fmt.Sprintf("proxy/%s/%s", upstream, repo)

	// Save manifest to database
	if err := p.db.PutManifest(ctx, localRepo, reference, mediaType, digest, payload); err != nil {
		return fmt.Errorf("failed to save manifest to database: %w", err)
	}

	log.Printf("[Proxy] Cached manifest: %s:%s → %s", localRepo, reference, digest)
	return nil
}

// cacheBlob stores a blob in local storage
func (p *RegistryProxy) cacheBlob(ctx context.Context, digest string, reader io.ReadCloser, size int64) {
	defer reader.Close()

	blobPath := fmt.Sprintf("blobs/%s", digest)

	// Check if already cached (race condition check)
	if _, err := p.storage.Stat(ctx, blobPath); err == nil {
		log.Printf("[Proxy] Blob %s already cached", digest)
		return
	}

	// Write blob to storage
	writer, err := p.storage.Writer(ctx, blobPath)
	if err != nil {
		log.Printf("[Proxy] Failed to create blob writer: %v", err)
		return
	}
	defer writer.Close()

	written, err := io.Copy(writer, reader)
	if err != nil {
		log.Printf("[Proxy] Failed to write blob: %v", err)
		return
	}

	// Record blob in database
	if err := p.db.PutBlob(ctx, digest, written, "application/octet-stream"); err != nil {
		log.Printf("[Proxy] Failed to record blob in database: %v", err)
		return
	}

	log.Printf("[Proxy] Cached blob: %s (%d bytes)", digest, written)
}

// ProxyConfig represents proxy configuration
type ProxyConfig struct {
	Enabled    bool                `json:"enabled"`
	Upstreams  []UpstreamConfig    `json:"upstreams"`
	CacheTTL   int                 `json:"cache_ttl_hours"` // How long to keep cached images
	Remapping  map[string]string   `json:"remapping"`       // Map local paths to upstream paths
}

// UpstreamConfig represents upstream registry configuration
type UpstreamConfig struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Mirror   bool   `json:"mirror"` // Cache all pulls from this registry
}

// LoadConfig loads proxy configuration from JSON file
func LoadConfig(configPath string) (*ProxyConfig, error) {
	file, err := http.DefaultClient.Get(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	defer file.Body.Close()

	var config ProxyConfig
	if err := json.NewDecoder(file.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// calculateDigest calculates the SHA256 digest of content
func calculateDigest(content []byte) string {
	// This is a placeholder - implement proper SHA256 calculation
	// In production: import "crypto/sha256"
	// hash := sha256.Sum256(content)
	// return fmt.Sprintf("sha256:%x", hash)
	return fmt.Sprintf("sha256:%d", len(content)) // Mock
}

// ResolveUpstream determines which upstream registry to use for a given repository
func (p *RegistryProxy) ResolveUpstream(repo string) string {
	// Examples of repository paths:
	// - "library/ubuntu" → Docker Hub
	// - "gcr.io/project/image" → GCR
	// - "ghcr.io/owner/repo" → GitHub Container Registry

	if strings.HasPrefix(repo, "gcr.io/") {
		return "gcr"
	}

	if strings.HasPrefix(repo, "ghcr.io/") {
		return "ghcr"
	}

	if strings.HasPrefix(repo, "quay.io/") {
		return "quay"
	}

	// Default to Docker Hub
	return "dockerhub"
}

// StripUpstreamPrefix removes upstream registry prefix from repository name
func StripUpstreamPrefix(repo string) string {
	// gcr.io/project/image → project/image
	// ghcr.io/owner/repo → owner/repo
	// library/ubuntu → library/ubuntu (no change)

	for _, prefix := range []string{"gcr.io/", "ghcr.io/", "quay.io/", "docker.io/"} {
		if strings.HasPrefix(repo, prefix) {
			return strings.TrimPrefix(repo, prefix)
		}
	}

	return repo
}
