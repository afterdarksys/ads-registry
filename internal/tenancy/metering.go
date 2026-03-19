package tenancy

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// MeteringService handles usage tracking for billing
type MeteringService struct {
	db    *sql.DB
	cache *meteringCache
}

// meteringCache stores in-memory bandwidth tracking before flushing to DB
type meteringCache struct {
	mu     sync.RWMutex
	events map[int][]*BandwidthEvent // tenant_id -> events
}

// BandwidthEvent represents a single bandwidth event
type BandwidthEvent struct {
	TenantID       int
	Direction      string // "ingress" or "egress"
	Bytes          int64
	RepositoryPath string
	ResourceType   string
	Digest         string
	UserID         *int
	IPAddress      string
	UserAgent      string
}

// NewMeteringService creates a new metering service
func NewMeteringService(db *sql.DB) *MeteringService {
	ms := &MeteringService{
		db: db,
		cache: &meteringCache{
			events: make(map[int][]*BandwidthEvent),
		},
	}

	// Start background flush goroutine
	go ms.startFlushWorker()

	return ms
}

// TrackBandwidth records a bandwidth event
func (s *MeteringService) TrackBandwidth(ctx context.Context, event *BandwidthEvent) error {
	// Add to cache
	s.cache.mu.Lock()
	s.cache.events[event.TenantID] = append(s.cache.events[event.TenantID], event)
	s.cache.mu.Unlock()

	return nil
}

// startFlushWorker periodically flushes cached events to database
func (s *MeteringService) startFlushWorker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := s.flushEvents(context.Background()); err != nil {
			// Log error but don't stop worker
			fmt.Printf("Failed to flush metering events: %v\n", err)
		}
	}
}

// flushEvents writes cached events to database
func (s *MeteringService) flushEvents(ctx context.Context) error {
	s.cache.mu.Lock()
	events := s.cache.events
	s.cache.events = make(map[int][]*BandwidthEvent)
	s.cache.mu.Unlock()

	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO tenant_bandwidth_events
		(tenant_id, direction, bytes, repository_path, resource_type, digest, user_id, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, tenantEvents := range events {
		for _, event := range tenantEvents {
			_, err := stmt.ExecContext(ctx,
				event.TenantID,
				event.Direction,
				event.Bytes,
				event.RepositoryPath,
				event.ResourceType,
				event.Digest,
				event.UserID,
				event.IPAddress,
				event.UserAgent,
			)
			if err != nil {
				return fmt.Errorf("failed to insert bandwidth event: %w", err)
			}
		}
	}

	return tx.Commit()
}

// GetCurrentUsage calculates real-time usage for a tenant
func (s *MeteringService) GetCurrentUsage(ctx context.Context, tenantID int, schemaName string) (*UsageMetrics, error) {
	// Calculate storage from tenant schema
	var storageBytes, blobCount, manifestCount, repoCount int64

	// Query blob storage (tenant-scoped)
	query := fmt.Sprintf(`
		SET search_path TO %s, public;
		SELECT
			COALESCE(SUM(size_bytes), 0) as storage_bytes,
			COUNT(*) as blob_count
		FROM blobs;
	`, schemaName)

	err := s.db.QueryRowContext(ctx, query).Scan(&storageBytes, &blobCount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate storage: %w", err)
	}

	// Query manifest count
	manifestQuery := fmt.Sprintf(`
		SET search_path TO %s, public;
		SELECT COUNT(*) FROM manifests;
	`, schemaName)
	err = s.db.QueryRowContext(ctx, manifestQuery).Scan(&manifestCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count manifests: %w", err)
	}

	// Query repository count
	repoQuery := fmt.Sprintf(`
		SET search_path TO %s, public;
		SELECT COUNT(*) FROM repositories;
	`, schemaName)
	err = s.db.QueryRowContext(ctx, repoQuery).Scan(&repoCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count repositories: %w", err)
	}

	// Calculate bandwidth (last 30 days)
	var ingressBytes, egressBytes int64
	bandwidthQuery := `
		SELECT
			COALESCE(SUM(CASE WHEN direction = 'ingress' THEN bytes ELSE 0 END), 0) as ingress,
			COALESCE(SUM(CASE WHEN direction = 'egress' THEN bytes ELSE 0 END), 0) as egress
		FROM tenant_bandwidth_events
		WHERE tenant_id = $1 AND recorded_at > NOW() - INTERVAL '30 days'
	`
	err = s.db.QueryRowContext(ctx, bandwidthQuery, tenantID).Scan(&ingressBytes, &egressBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate bandwidth: %w", err)
	}

	return &UsageMetrics{
		TenantID:              tenantID,
		StorageBytes:          storageBytes,
		BlobCount:             int(blobCount),
		ManifestCount:         int(manifestCount),
		RepositoryCount:       int(repoCount),
		BandwidthIngressBytes: ingressBytes,
		BandwidthEgressBytes:  egressBytes,
	}, nil
}

// AggregateUsageMetrics creates a usage metrics record for a time period
func (s *MeteringService) AggregateUsageMetrics(ctx context.Context, tenantID int, schemaName string, periodStart, periodEnd time.Time) error {
	// Get current usage snapshot
	usage, err := s.GetCurrentUsage(ctx, tenantID, schemaName)
	if err != nil {
		return fmt.Errorf("failed to get current usage: %w", err)
	}

	// Insert aggregated metrics
	query := `
		INSERT INTO tenant_usage_metrics
		(tenant_id, period_start, period_end, storage_bytes, storage_bytes_max,
		 blob_count, manifest_count, repository_count,
		 bandwidth_ingress_bytes, bandwidth_egress_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, period_start, period_end)
		DO UPDATE SET
			storage_bytes = EXCLUDED.storage_bytes,
			storage_bytes_max = GREATEST(tenant_usage_metrics.storage_bytes_max, EXCLUDED.storage_bytes),
			blob_count = EXCLUDED.blob_count,
			manifest_count = EXCLUDED.manifest_count,
			repository_count = EXCLUDED.repository_count,
			bandwidth_ingress_bytes = EXCLUDED.bandwidth_ingress_bytes,
			bandwidth_egress_bytes = EXCLUDED.bandwidth_egress_bytes,
			updated_at = NOW()
	`

	_, err = s.db.ExecContext(ctx, query,
		tenantID,
		periodStart,
		periodEnd,
		usage.StorageBytes,
		usage.StorageBytes, // max = current for now
		usage.BlobCount,
		usage.ManifestCount,
		usage.RepositoryCount,
		usage.BandwidthIngressBytes,
		usage.BandwidthEgressBytes,
	)

	return err
}

// BandwidthMeteringMiddleware tracks bandwidth for all requests
type BandwidthMeteringMiddleware struct {
	metering *MeteringService
}

// NewBandwidthMeteringMiddleware creates bandwidth tracking middleware
func NewBandwidthMeteringMiddleware(metering *MeteringService) *BandwidthMeteringMiddleware {
	return &BandwidthMeteringMiddleware{metering: metering}
}

// Middleware wraps HTTP handlers to track bandwidth
func (m *BandwidthMeteringMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get tenant from context
		tenant, ok := GetTenantFromContext(r.Context())
		if !ok {
			// No tenant context, skip metering
			next.ServeHTTP(w, r)
			return
		}

		// Wrap response writer to capture bytes written
		mrw := &meteringResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Track ingress (request body)
		if r.ContentLength > 0 {
			event := &BandwidthEvent{
				TenantID:  tenant.ID,
				Direction: "ingress",
				Bytes:     r.ContentLength,
				IPAddress: r.RemoteAddr,
				UserAgent: r.UserAgent(),
			}
			m.metering.TrackBandwidth(r.Context(), event)
		}

		// Process request
		next.ServeHTTP(mrw, r)

		// Track egress (response body)
		if mrw.bytesWritten > 0 {
			event := &BandwidthEvent{
				TenantID:  tenant.ID,
				Direction: "egress",
				Bytes:     mrw.bytesWritten,
				IPAddress: r.RemoteAddr,
				UserAgent: r.UserAgent(),
			}
			m.metering.TrackBandwidth(r.Context(), event)
		}
	})
}

// meteringResponseWriter wraps http.ResponseWriter to track bytes written
type meteringResponseWriter struct {
	http.ResponseWriter
	bytesWritten int64
	statusCode   int
}

func (mrw *meteringResponseWriter) Write(b []byte) (int, error) {
	n, err := mrw.ResponseWriter.Write(b)
	mrw.bytesWritten += int64(n)
	return n, err
}

func (mrw *meteringResponseWriter) WriteHeader(statusCode int) {
	mrw.statusCode = statusCode
	mrw.ResponseWriter.WriteHeader(statusCode)
}
