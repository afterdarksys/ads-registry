package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ryan/ads-registry/internal/automation"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

// Manager handles background synchronization of images to peer registries
type Manager struct {
	peers       []config.PeerRegistry
	starlark    *automation.Engine
	jobQueue    chan SyncJob
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	db          db.Store
	storage     storage.Provider
	localHost   string // Local registry hostname for pulling content
	httpClient  *http.Client
	metrics     *SyncMetrics
	metricsMu   sync.RWMutex
}

// SyncMetrics tracks synchronization statistics for monitoring
type SyncMetrics struct {
	TotalJobs       int64
	SuccessfulJobs  int64
	FailedJobs      int64
	TotalBytesSync  int64
	LastSyncTime    time.Time
	AverageLatency  time.Duration
}

type SyncJob struct {
	Namespace  string
	Repository string
	Reference  string
	Digest     string
}

func NewManager(cfg []config.PeerRegistry, starlarkEngine *automation.Engine, dbStore db.Store, storageProvider storage.Provider, localHost string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	// Create HTTP client with reasonable timeouts and connection pooling
	httpClient := &http.Client{
		Timeout: 10 * time.Minute, // Allow time for large layer uploads
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true, // Layers are already compressed
		},
	}

	return &Manager{
		peers:      cfg,
		starlark:   starlarkEngine,
		jobQueue:   make(chan SyncJob, 1000), // Buffer for high throughput pushes
		ctx:        ctx,
		cancel:     cancel,
		db:         dbStore,
		storage:    storageProvider,
		localHost:  localHost,
		httpClient: httpClient,
		metrics:    &SyncMetrics{},
	}
}

// Start begins the background worker pool
func (m *Manager) Start(workers int) {
	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go m.worker()
	}
	log.Printf("[SyncManager] Started %d synchronization background workers", workers)
}

// Stop gracefully shuts down sync processes
func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
}

// EnqueuePush schedules a successful local push to be synced to peers
func (m *Manager) EnqueuePush(namespace, repo, ref, digest string) {
	select {
	case m.jobQueue <- SyncJob{
		Namespace:  namespace,
		Repository: repo,
		Reference:  ref,
		Digest:     digest,
	}:
		// queued
	default:
		log.Printf("[SyncManager] WARNING: Job queue full, dropping sync event for %s", repo)
	}
}

func (m *Manager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case job := <-m.jobQueue:
			m.processJob(job)
		}
	}
}

func (m *Manager) processJob(job SyncJob) {
	for _, peer := range m.peers {
		if peer.Mode != "push" && peer.Mode != "bidirectional" {
			continue
		}

		// Evaluate Starlark Policy to see if we're allowed to sync to this peer
		allowed := true
		if m.starlark != nil {
			eventPayload := map[string]string{
				"namespace":  job.Namespace,
				"repository": job.Repository,
				"reference":  job.Reference,
				"digest":     job.Digest,
				"peer_name":  peer.Name,
				"peer_url":   peer.Endpoint,
			}
			var err error
			allowed, err = m.starlark.EvaluateSyncPolicy("scripts/sync_policy.star", eventPayload)
			if err != nil {
				log.Printf("[SyncManager] Starlark policy error for %s: %v", peer.Name, err)
				// Fail-safe: if script exists but errors, block it. If script missing, default allow?
				// For compliance safety, better to block if policy errors. But here we assume missing = false error.
				allowed = false 
			}
		}

		if !allowed {
			log.Printf("[SyncManager] Sync blocked by Starlark policy to peer %s for image %s:%s", peer.Name, job.Repository, job.Reference)
			continue
		}

		// Perform the sync with retry logic
		log.Printf("[SyncManager] Authorized sync pushing %s:%s to %s", job.Repository, job.Reference, peer.Name)

		startTime := time.Now()
		err := m.syncImageToPeer(job, peer)
		duration := time.Since(startTime)

		m.updateMetrics(err == nil, duration)

		if err != nil {
			log.Printf("[SyncManager] ERROR: Failed to sync %s:%s to %s after retries: %v",
				job.Repository, job.Reference, peer.Name, err)
		} else {
			log.Printf("[SyncManager] SUCCESS: Synced %s:%s to %s in %v",
				job.Repository, job.Reference, peer.Name, duration)
		}
	}
}

// syncImageToPeer implements the complete sync workflow with retry logic
func (m *Manager) syncImageToPeer(job SyncJob, peer config.PeerRegistry) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			// Exponential backoff: 2^attempt seconds (2s, 4s, 8s)
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			log.Printf("[SyncManager] Retry %d/%d for %s:%s to %s after %v",
				attempt, maxRetries, job.Repository, job.Reference, peer.Name, backoff)

			select {
			case <-time.After(backoff):
			case <-m.ctx.Done():
				return fmt.Errorf("sync cancelled during backoff")
			}
		}

		err := m.performSync(job, peer)
		if err == nil {
			return nil // Success
		}

		lastErr = err
		log.Printf("[SyncManager] Attempt %d/%d failed: %v", attempt, maxRetries, err)
	}

	return fmt.Errorf("sync failed after %d attempts: %w", maxRetries, lastErr)
}

// performSync executes a single sync attempt
func (m *Manager) performSync(job SyncJob, peer config.PeerRegistry) error {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	defer cancel()

	// Step 1: Fetch manifest from local registry
	manifest, mediaType, err := m.fetchLocalManifest(ctx, job.Repository, job.Digest)
	if err != nil {
		return fmt.Errorf("fetch local manifest: %w", err)
	}

	// Step 2: Parse manifest to get layer digests
	layerDigests, configDigest, err := m.parseManifestLayers(manifest, mediaType)
	if err != nil {
		return fmt.Errorf("parse manifest layers: %w", err)
	}

	// Step 3: Sync all blobs (config + layers) to peer
	allDigests := append([]string{configDigest}, layerDigests...)
	for _, blobDigest := range allDigests {
		if blobDigest == "" {
			continue
		}

		if err := m.syncBlob(ctx, blobDigest, job.Repository, peer); err != nil {
			return fmt.Errorf("sync blob %s: %w", blobDigest, err)
		}
	}

	// Step 4: Push manifest to peer
	if err := m.pushManifestToPeer(ctx, job.Repository, job.Reference, manifest, mediaType, peer); err != nil {
		return fmt.Errorf("push manifest to peer: %w", err)
	}

	return nil
}

// fetchLocalManifest retrieves a manifest from the local database
func (m *Manager) fetchLocalManifest(ctx context.Context, repo, digest string) ([]byte, string, error) {
	mediaType, manifestDigest, payload, err := m.db.GetManifest(ctx, repo, digest)
	if err != nil {
		return nil, "", fmt.Errorf("db.GetManifest: %w", err)
	}

	log.Printf("[SyncManager] Fetched local manifest %s (type: %s, size: %d bytes)",
		manifestDigest, mediaType, len(payload))

	return payload, mediaType, nil
}

// parseManifestLayers extracts layer and config digests from a manifest
func (m *Manager) parseManifestLayers(manifestData []byte, mediaType string) (layers []string, config string, err error) {
	var manifest map[string]interface{}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, "", fmt.Errorf("unmarshal manifest: %w", err)
	}

	// Handle different manifest types
	switch {
	case strings.Contains(mediaType, "manifest.v2") || strings.Contains(mediaType, "oci.image.manifest"):
		// Docker Image Manifest V2, Schema 2 or OCI Image Manifest
		if configObj, ok := manifest["config"].(map[string]interface{}); ok {
			if digestStr, ok := configObj["digest"].(string); ok {
				config = digestStr
			}
		}

		if layersArray, ok := manifest["layers"].([]interface{}); ok {
			for _, layer := range layersArray {
				if layerMap, ok := layer.(map[string]interface{}); ok {
					if digestStr, ok := layerMap["digest"].(string); ok {
						layers = append(layers, digestStr)
					}
				}
			}
		}

	case strings.Contains(mediaType, "manifest.list") || strings.Contains(mediaType, "oci.image.index"):
		// Manifest List (multi-arch) - we need to recursively sync all manifests
		if manifestsArray, ok := manifest["manifests"].([]interface{}); ok {
			for _, mani := range manifestsArray {
				if maniMap, ok := mani.(map[string]interface{}); ok {
					if digestStr, ok := maniMap["digest"].(string); ok {
						// For manifest lists, treat sub-manifests as "layers"
						layers = append(layers, digestStr)
					}
				}
			}
		}

	default:
		log.Printf("[SyncManager] WARNING: Unknown manifest type %s, attempting generic parse", mediaType)
	}

	log.Printf("[SyncManager] Parsed manifest: config=%s, layers=%d", config, len(layers))
	return layers, config, nil
}

// syncBlob ensures a blob exists on the peer registry
func (m *Manager) syncBlob(ctx context.Context, blobDigest, repo string, peer config.PeerRegistry) error {
	// Step 1: Check if blob already exists on peer (HEAD request)
	exists, err := m.blobExistsOnPeer(ctx, blobDigest, repo, peer)
	if err != nil {
		log.Printf("[SyncManager] Warning: Failed to check blob existence on peer: %v (will attempt upload)", err)
		exists = false
	}

	if exists {
		log.Printf("[SyncManager] Blob %s already exists on peer %s, skipping", blobDigest, peer.Name)
		return nil
	}

	// Step 2: Read blob from local storage
	blobPath := m.getBlobPath(blobDigest)
	reader, err := m.storage.Reader(ctx, blobPath, 0)
	if err != nil {
		return fmt.Errorf("read local blob: %w", err)
	}
	defer reader.Close()

	// Step 3: Get blob size for progress tracking
	size, err := m.storage.Stat(ctx, blobPath)
	if err != nil {
		return fmt.Errorf("stat blob: %w", err)
	}

	// Step 4: Upload blob to peer
	if err := m.uploadBlobToPeer(ctx, reader, blobDigest, size, repo, peer); err != nil {
		return fmt.Errorf("upload blob to peer: %w", err)
	}

	log.Printf("[SyncManager] Successfully synced blob %s (%d bytes) to %s", blobDigest, size, peer.Name)
	return nil
}

// blobExistsOnPeer checks if a blob exists on the peer registry using HEAD request
func (m *Manager) blobExistsOnPeer(ctx context.Context, blobDigest, repo string, peer config.PeerRegistry) (bool, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", strings.TrimSuffix(peer.Endpoint, "/"), repo, blobDigest)

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", "Bearer "+peer.Token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// uploadBlobToPeer uploads a blob to the peer registry using chunked upload
func (m *Manager) uploadBlobToPeer(ctx context.Context, reader io.Reader, blobDigest string, size int64, repo string, peer config.PeerRegistry) error {
	// Step 1: Initiate upload session
	uploadURL, err := m.initiateUpload(ctx, repo, peer)
	if err != nil {
		return fmt.Errorf("initiate upload: %w", err)
	}

	// Step 2: Upload blob data
	// For simplicity, we'll do a monolithic upload. For production, consider chunked uploads for very large blobs
	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, reader)
	if err != nil {
		return err
	}

	// Add digest as query parameter
	if !strings.Contains(uploadURL, "?") {
		uploadURL += "?digest=" + blobDigest
	} else {
		uploadURL += "&digest=" + blobDigest
	}
	req.URL, _ = req.URL.Parse(uploadURL)

	req.Header.Set("Authorization", "Bearer "+peer.Token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", size))

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// initiateUpload starts a blob upload session and returns the upload URL
func (m *Manager) initiateUpload(ctx context.Context, repo string, peer config.PeerRegistry) (string, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/uploads/", strings.TrimSuffix(peer.Endpoint, "/"), repo)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+peer.Token)
	req.Header.Set("Content-Length", "0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("initiate upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("initiate upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Extract upload URL from Location header
	uploadURL := resp.Header.Get("Location")
	if uploadURL == "" {
		return "", fmt.Errorf("no Location header in upload initiation response")
	}

	// Make URL absolute if it's relative
	if strings.HasPrefix(uploadURL, "/") {
		uploadURL = strings.TrimSuffix(peer.Endpoint, "/") + uploadURL
	}

	return uploadURL, nil
}

// pushManifestToPeer uploads a manifest to the peer registry
func (m *Manager) pushManifestToPeer(ctx context.Context, repo, reference string, manifest []byte, mediaType string, peer config.PeerRegistry) error {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", strings.TrimSuffix(peer.Endpoint, "/"), repo, reference)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(manifest))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+peer.Token)
	req.Header.Set("Content-Type", mediaType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(manifest)))

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("push manifest request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push manifest failed with status %d: %s", resp.StatusCode, string(body))
	}

	manifestDigest := resp.Header.Get("Docker-Content-Digest")
	log.Printf("[SyncManager] Pushed manifest %s:%s to %s (digest: %s)", repo, reference, peer.Name, manifestDigest)

	return nil
}

// getBlobPath constructs the storage path for a blob digest
func (m *Manager) getBlobPath(blobDigest string) string {
	// Parse digest (format: sha256:abc123...)
	parts := strings.SplitN(blobDigest, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	algorithm := parts[0]
	hash := parts[1]

	// Standard Docker registry layout: blobs/<algorithm>/<first-two-chars>/<hash>/data
	return fmt.Sprintf("blobs/%s/%s/%s/data", algorithm, hash[:2], hash)
}

// updateMetrics updates sync metrics for monitoring
func (m *Manager) updateMetrics(success bool, duration time.Duration) {
	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()

	m.metrics.TotalJobs++
	if success {
		m.metrics.SuccessfulJobs++
	} else {
		m.metrics.FailedJobs++
	}

	m.metrics.LastSyncTime = time.Now()

	// Update rolling average latency
	if m.metrics.AverageLatency == 0 {
		m.metrics.AverageLatency = duration
	} else {
		m.metrics.AverageLatency = (m.metrics.AverageLatency + duration) / 2
	}
}

// GetMetrics returns current sync metrics (for monitoring/dashboards)
func (m *Manager) GetMetrics() SyncMetrics {
	m.metricsMu.RLock()
	defer m.metricsMu.RUnlock()
	return *m.metrics
}
