package compat

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"
)

// Middleware provides compatibility workarounds for broken container registry clients
type Middleware struct {
	config   *Config
	detector *ClientDetector
	metrics  *Metrics
}

// NewMiddleware creates a new compatibility middleware
func NewMiddleware(config *Config) (*Middleware, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid compatibility config: %w", err)
	}

	metrics := NewMetrics(config.Observability.MetricsPrefix)

	return &Middleware{
		config:   config,
		detector: NewClientDetector(),
		metrics:  metrics,
	}, nil
}

// ClientDetectionMiddleware detects the client and adds ClientInfo to context
// This should run early in the middleware chain
func (m *Middleware) ClientDetectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if compatibility system is disabled
		if !m.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Detect client
		clientInfo := m.detector.DetectClient(r)

		// Record metrics
		if m.config.Observability.EnableMetrics {
			m.metrics.RecordClientDetection(clientInfo.Name, clientInfo.Version, clientInfo.Protocol)
		}

		// Log detection
		if m.config.Observability.LogClientDetection && m.shouldLog() {
			log.Printf("[COMPAT] Client detected: %s/%s (protocol=%s, http=%s, ua=%s)",
				clientInfo.Name, clientInfo.Version, clientInfo.Protocol,
				clientInfo.HTTPVersion, clientInfo.UserAgent)
		}

		// Add to context
		ctx := WithClientInfo(r.Context(), clientInfo)

		// Continue with enriched context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CompatibilityMiddleware applies workarounds based on detected client
// This should run after ClientDetectionMiddleware
func (m *Middleware) CompatibilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if compatibility system is disabled
		if !m.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Get client info from context
		clientInfo := GetClientInfo(r.Context())
		if clientInfo == nil {
			// Client detection middleware didn't run, continue without workarounds
			next.ServeHTTP(w, r)
			return
		}

		// Check if we need to apply workarounds
		workarounds := m.determineWorkarounds(clientInfo, r)

		if len(workarounds) == 0 {
			// No workarounds needed
			next.ServeHTTP(w, r)
			return
		}

		// Apply workarounds
		m.applyWorkarounds(w, r, clientInfo, workarounds, next)
	})
}

// determineWorkarounds checks which workarounds should be activated
func (m *Middleware) determineWorkarounds(client *ClientInfo, r *http.Request) []string {
	var workarounds []string

	// Docker manifest upload fix (affects 18.x, 19.x, 29.x)
	if m.config.DockerClientWorkarounds.EnableDocker29ManifestFix {
		if client.IsDockerClient() && (client.MatchesVersion("18.*") || client.MatchesVersion("19.*") || client.MatchesVersion("29.*")) {
			if r.Method == "PUT" && strings.Contains(r.URL.Path, "/manifests/") {
				workarounds = append(workarounds, "docker_29_manifest_fix")
			}
			// Force connection closure on blob operations to prevent reuse issues
			if strings.Contains(r.URL.Path, "/blobs/") {
				workarounds = append(workarounds, "docker_29_force_close_blobs")
			}
		}
	}

	// Force HTTP/1.1 for manifests
	if m.config.DockerClientWorkarounds.ForceHTTP1ForManifests {
		if strings.Contains(r.URL.Path, "/manifests/") {
			if client.SupportsHTTP2() {
				workarounds = append(workarounds, "force_http1_manifests")
			}
		}
	}

	// Podman digest workaround
	if m.config.BrokenClientHacks.PodmanDigestWorkaround {
		if client.IsPodmanClient() && client.MatchesVersion("3.*") {
			if strings.Contains(r.URL.Path, "/blobs/") {
				workarounds = append(workarounds, "podman_digest_fix")
			}
		}
	}

	// Containerd Content-Length workaround
	if m.config.BrokenClientHacks.ContainerdContentLength {
		if client.IsContainerdClient() && client.MatchesVersion("1.6.*") {
			workarounds = append(workarounds, "containerd_content_length")
		}
	}

	// Header workarounds
	if m.config.HeaderWorkarounds.AlwaysSendDistributionAPIVersion {
		workarounds = append(workarounds, "distribution_api_version")
	}

	return workarounds
}

// applyWorkarounds applies the determined workarounds to the request/response
func (m *Middleware) applyWorkarounds(w http.ResponseWriter, r *http.Request,
	client *ClientInfo, workarounds []string, next http.Handler) {

	start := time.Now()

	// Wrap the response writer with workaround capabilities
	wrappedWriter := m.wrapResponseWriter(w, client, workarounds)

	// Log workaround activation
	if m.config.Observability.LogWorkarounds && m.shouldLog() {
		log.Printf("[COMPAT] Activating workarounds for %s: %v (path=%s)",
			client.String(), workarounds, r.URL.Path)
	}

	// Record metrics
	for _, wa := range workarounds {
		client.AddWorkaround(wa)

		if m.config.Observability.EnableMetrics {
			m.metrics.RecordWorkaroundActivation(client.Name, client.Version, wa)
		}
	}

	// Call next handler with wrapped writer
	next.ServeHTTP(wrappedWriter, r)

	// Record duration
	duration := time.Since(start).Seconds()
	for _, wa := range workarounds {
		if m.config.Observability.EnableMetrics {
			m.metrics.ObserveWorkaroundDuration(wa, duration)
		}
	}
}

// wrapResponseWriter wraps http.ResponseWriter with workaround capabilities
func (m *Middleware) wrapResponseWriter(w http.ResponseWriter, client *ClientInfo,
	workarounds []string) http.ResponseWriter {

	wrapper := &compatResponseWriter{
		ResponseWriter: w,
		middleware:     m,
		client:         client,
		workarounds:    workarounds,
		extraFlushes:   m.config.DockerClientWorkarounds.ExtraFlushes,
		headerDelay:    time.Duration(m.config.DockerClientWorkarounds.HeaderWriteDelay) * time.Millisecond,
	}

	return wrapper
}

// shouldLog determines if this request should be logged based on sample rate
func (m *Middleware) shouldLog() bool {
	if m.config.Observability.LogSampleRate >= 1.0 {
		return true
	}
	return rand.Float64() < m.config.Observability.LogSampleRate
}

// compatResponseWriter wraps http.ResponseWriter to apply workarounds
type compatResponseWriter struct {
	http.ResponseWriter
	middleware   *Middleware
	client       *ClientInfo
	workarounds  []string
	extraFlushes int
	headerDelay  time.Duration
	wroteHeader  bool
	statusCode   int
}

// WriteHeader intercepts header writes to apply workarounds
func (w *compatResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}

	w.statusCode = statusCode
	w.wroteHeader = true

	// Apply header workarounds
	w.applyHeaderWorkarounds()

	// Write the actual header
	w.ResponseWriter.WriteHeader(statusCode)

	// Docker 29.x manifest fix: Add delay after header write
	if w.hasWorkaround("docker_29_manifest_fix") && w.headerDelay > 0 {
		time.Sleep(w.headerDelay)
	}

	// Docker 29.x manifest fix: Extra flush after headers
	if w.hasWorkaround("docker_29_manifest_fix") {
		if f, ok := w.ResponseWriter.(http.Flusher); ok {
			f.Flush()

			// Record metric
			if w.middleware.config.Observability.EnableMetrics {
				w.middleware.metrics.RecordManifestFix(w.client.Name, "header_flush")
			}
		}
	}
}

// Write intercepts body writes to apply workarounds
func (w *compatResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	// Write the data
	n, err := w.ResponseWriter.Write(b)

	// Docker 29.x manifest fix: Extra flushes after write
	if w.hasWorkaround("docker_29_manifest_fix") && w.extraFlushes > 0 {
		if f, ok := w.ResponseWriter.(http.Flusher); ok {
			for i := 0; i < w.extraFlushes; i++ {
				f.Flush()
			}

			// Record metric
			if w.middleware.config.Observability.EnableMetrics {
				w.middleware.metrics.RecordManifestFix(w.client.Name, "extra_flush")
			}
		}
	}

	return n, err
}

// applyHeaderWorkarounds applies header-specific workarounds
func (w *compatResponseWriter) applyHeaderWorkarounds() {
	header := w.ResponseWriter.Header()

	// Always send Distribution API version
	if w.hasWorkaround("distribution_api_version") {
		if header.Get("Docker-Distribution-API-Version") == "" {
			header.Set("Docker-Distribution-API-Version", "registry/2.0")

			if w.middleware.config.Observability.EnableMetrics {
				w.middleware.metrics.RecordHeaderWorkaround("Docker-Distribution-API-Version", "force_add")
			}
		}
	}

	// Containerd Content-Length workaround
	if w.hasWorkaround("containerd_content_length") {
		// Ensure Content-Length is always set (even if 0)
		if header.Get("Content-Length") == "" {
			header.Set("Content-Length", "0")

			if w.middleware.config.Observability.EnableMetrics {
				w.middleware.metrics.RecordHeaderWorkaround("Content-Length", "force_add")
			}
		}
	}

	// Content-Type fixups
	if w.middleware.config.HeaderWorkarounds.ContentTypeFixups {
		contentType := header.Get("Content-Type")
		if contentType != "" {
			// Normalize OCI vs Docker media types based on client
			normalizedType := w.normalizeContentType(contentType)
			if normalizedType != contentType {
				header.Set("Content-Type", normalizedType)

				if w.middleware.config.Observability.EnableMetrics {
					w.middleware.metrics.RecordHeaderWorkaround("Content-Type", "normalize")
				}
			}
		}
	}

	// CORS headers
	if w.middleware.config.HeaderWorkarounds.EnableCORS {
		header.Set("Access-Control-Allow-Origin", "*")
		header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	}

	// Docker 29.x connection closure fix for blob operations
	if w.hasWorkaround("docker_29_force_close_blobs") {
		header.Set("Connection", "close")

		if w.middleware.config.Observability.EnableMetrics {
			w.middleware.metrics.RecordHeaderWorkaround("Connection", "force_close")
		}
	}
}

// normalizeContentType normalizes media types based on client preferences
func (w *compatResponseWriter) normalizeContentType(contentType string) string {
	// Docker clients prefer Docker media types
	if w.client.Protocol == "docker" {
		if strings.Contains(contentType, "vnd.oci.image") {
			return strings.ReplaceAll(contentType, "vnd.oci.image", "vnd.docker.distribution")
		}
	}

	// OCI clients prefer OCI media types
	if w.client.Protocol == "oci" {
		if strings.Contains(contentType, "vnd.docker.distribution") {
			return strings.ReplaceAll(contentType, "vnd.docker.distribution", "vnd.oci.image")
		}
	}

	return contentType
}

// hasWorkaround checks if a specific workaround is active
func (w *compatResponseWriter) hasWorkaround(name string) bool {
	for _, wa := range w.workarounds {
		if wa == name {
			return true
		}
	}
	return false
}

// Flush implements http.Flusher
func (w *compatResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker
func (w *compatResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Push implements http.Pusher for HTTP/2 server push
func (w *compatResponseWriter) Push(target string, opts *http.PushOptions) error {
	// Disable server push for clients with HTTP/2 workarounds
	if w.hasWorkaround("force_http1_manifests") {
		return http.ErrNotSupported
	}

	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
