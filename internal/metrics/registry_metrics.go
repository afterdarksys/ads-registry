package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry Metrics for comprehensive observability

var (
	// === Image Operations ===

	// ImagePushes tracks image push operations
	ImagePushes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_image_pushes_total",
			Help: "Total number of image pushes",
		},
		[]string{"namespace", "repository", "status"}, // status: success, failure
	)

	// ImagePulls tracks image pull operations
	ImagePulls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_image_pulls_total",
			Help: "Total number of image pulls",
		},
		[]string{"namespace", "repository", "status"},
	)

	// ImageDeletes tracks image deletion operations
	ImageDeletes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_image_deletes_total",
			Help: "Total number of image deletions",
		},
		[]string{"namespace", "repository", "status"},
	)

	// === Storage Metrics ===

	// StorageUsage tracks total storage used
	StorageUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_storage_bytes",
			Help: "Total storage used in bytes",
		},
		[]string{"namespace", "type"}, // type: blobs, manifests
	)

	// BlobSize tracks individual blob sizes
	BlobSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "registry_blob_size_bytes",
			Help:    "Size of uploaded blobs in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 20), // 1KB to 512MB
		},
	)

	// === Security Scanning ===

	// ScanDuration tracks scan execution time
	ScanDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "registry_scan_duration_seconds",
			Help:    "Duration of security scans",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"scanner"}, // scanner: trivy, semgrep, clamav
	)

	// ScanFindings tracks vulnerability findings
	ScanFindings = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_scan_findings_total",
			Help: "Total number of scan findings",
		},
		[]string{"scanner", "severity"}, // severity: critical, high, medium, low
	)

	// VulnerableImages tracks images with vulnerabilities
	VulnerableImages = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_vulnerable_images",
			Help: "Number of images with vulnerabilities",
		},
		[]string{"severity"},
	)

	// MalwareDetections tracks malware detections
	MalwareDetections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_malware_detections_total",
			Help: "Total number of malware detections",
		},
		[]string{"threat_type"}, // threat_type: trojan, backdoor, virus, etc.
	)

	// === Pull-Through Cache ===

	// ProxyCacheHits tracks cache hits
	ProxyCacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_proxy_cache_hits_total",
			Help: "Total number of proxy cache hits",
		},
		[]string{"upstream"}, // upstream: dockerhub, gcr, ghcr
	)

	// ProxyCacheMisses tracks cache misses
	ProxyCacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_proxy_cache_misses_total",
			Help: "Total number of proxy cache misses",
		},
		[]string{"upstream"},
	)

	// ProxyCachedBytes tracks cached data
	ProxyCachedBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_proxy_cached_bytes",
			Help: "Total bytes cached from upstream registries",
		},
		[]string{"upstream"},
	)

	// UpstreamFetchDuration tracks upstream fetch latency
	UpstreamFetchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "registry_upstream_fetch_duration_seconds",
			Help:    "Duration of upstream registry fetches",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"upstream", "type"}, // type: manifest, blob
	)

	// === API Performance ===

	// HTTPRequestDuration tracks request latency
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "registry_http_request_duration_seconds",
			Help:    "HTTP request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestSize tracks request size
	HTTPRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "registry_http_request_size_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path"},
	)

	// HTTPResponseSize tracks response size
	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "registry_http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path"},
	)

	// === Authentication & Authorization ===

	// AuthenticationAttempts tracks login attempts
	AuthenticationAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_authentication_attempts_total",
			Help: "Total number of authentication attempts",
		},
		[]string{"result"}, // result: success, failure
	)

	// ActiveSessions tracks active user sessions
	ActiveSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "registry_active_sessions",
			Help: "Number of active user sessions",
		},
	)

	// TokenGenerations tracks token generation
	TokenGenerations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_token_generations_total",
			Help: "Total number of tokens generated",
		},
		[]string{"type"}, // type: imagepullsecret, kubelet, api
	)

	// === Database ===

	// DatabaseConnections tracks DB connection pool
	DatabaseConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_database_connections",
			Help: "Number of database connections",
		},
		[]string{"state"}, // state: active, idle
	)

	// DatabaseQueryDuration tracks query performance
	DatabaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "registry_database_query_duration_seconds",
			Help:    "Database query duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"}, // operation: select, insert, update, delete
	)

	// === Queue (River) ===

	// QueuedJobs tracks jobs in queue
	QueuedJobs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_queue_jobs",
			Help: "Number of jobs in queue",
		},
		[]string{"queue", "state"}, // state: pending, running, completed, failed
	)

	// JobDuration tracks job execution time
	JobDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "registry_job_duration_seconds",
			Help:    "Job execution duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"job_type"}, // job_type: scan, rescan, token_refresh
	)

	// === Garbage Collection ===

	// GarbageCollectionRuns tracks GC executions
	GarbageCollectionRuns = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_gc_runs_total",
			Help: "Total number of garbage collection runs",
		},
	)

	// GarbageCollectionDuration tracks GC duration
	GarbageCollectionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "registry_gc_duration_seconds",
			Help:    "Garbage collection duration",
			Buckets: prometheus.DefBuckets,
		},
	)

	// GarbageCollectionReclaimed tracks reclaimed space
	GarbageCollectionReclaimed = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_gc_reclaimed_bytes_total",
			Help: "Total bytes reclaimed by garbage collection",
		},
	)

	// === Notifications ===

	// NotificationsSent tracks notifications
	NotificationsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_notifications_sent_total",
			Help: "Total number of notifications sent",
		},
		[]string{"channel", "type"}, // channel: email, slack, webhook; type: scan_results, alert
	)

	// NotificationErrors tracks notification failures
	NotificationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_notification_errors_total",
			Help: "Total number of notification errors",
		},
		[]string{"channel"},
	)

	// === Webhooks ===

	// WebhookDeliveries tracks webhook deliveries
	WebhookDeliveries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_webhook_deliveries_total",
			Help: "Total number of webhook deliveries",
		},
		[]string{"event", "status"}, // event: image.pushed, image.deleted; status: success, failure
	)

	// WebhookDuration tracks webhook delivery time
	WebhookDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "registry_webhook_duration_seconds",
			Help:    "Webhook delivery duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"event"},
	)

	// === Quota & Rate Limiting ===

	// QuotaUsage tracks namespace quota usage
	QuotaUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_quota_usage_bytes",
			Help: "Quota usage by namespace",
		},
		[]string{"namespace"},
	)

	// QuotaLimit tracks namespace quota limits
	QuotaLimit = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_quota_limit_bytes",
			Help: "Quota limit by namespace",
		},
		[]string{"namespace"},
	)

	// RateLimitExceeded tracks rate limit violations
	RateLimitExceeded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_rate_limit_exceeded_total",
			Help: "Total number of rate limit violations",
		},
		[]string{"endpoint"},
	)

	// === Replication (if enabled) ===

	// ReplicationLag tracks replication delay
	ReplicationLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_replication_lag_seconds",
			Help: "Replication lag to remote registry",
		},
		[]string{"remote"},
	)

	// ReplicationErrors tracks replication failures
	ReplicationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registry_replication_errors_total",
			Help: "Total number of replication errors",
		},
		[]string{"remote", "operation"},
	)

	// === Build Info ===

	// BuildInfo provides version and build information
	BuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "registry_build_info",
			Help: "Registry build information",
		},
		[]string{"version", "commit", "build_date"},
	)
)

// SetBuildInfo sets the build information metric
func SetBuildInfo(version, commit, buildDate string) {
	BuildInfo.WithLabelValues(version, commit, buildDate).Set(1)
}
