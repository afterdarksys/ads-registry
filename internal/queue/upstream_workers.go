package queue

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/ryan/ads-registry/internal/upstreams"
)

// TokenRefreshJobArgs represents arguments for refreshing a single upstream token
type TokenRefreshJobArgs struct {
	UpstreamID int `json:"upstream_id"`
}

// Kind returns the unique identifier for this job type
func (TokenRefreshJobArgs) Kind() string {
	return "upstream_token_refresh"
}

// TokenRefreshWorker processes upstream token refresh jobs
type TokenRefreshWorker struct {
	river.WorkerDefaults[TokenRefreshJobArgs]
	manager *upstreams.Manager
}

// NewTokenRefreshWorker creates a new token refresh worker
func NewTokenRefreshWorker(manager *upstreams.Manager) *TokenRefreshWorker {
	return &TokenRefreshWorker{
		manager: manager,
	}
}

// Work processes a single token refresh job
func (w *TokenRefreshWorker) Work(ctx context.Context, job *river.Job[TokenRefreshJobArgs]) error {
	args := job.Args

	log.Printf("[River] Refreshing token for upstream ID: %d", args.UpstreamID)

	if err := w.manager.RefreshUpstreamToken(ctx, args.UpstreamID); err != nil {
		log.Printf("[River] Failed to refresh token for upstream %d: %v", args.UpstreamID, err)
		return err
	}

	log.Printf("[River] Successfully refreshed token for upstream ID: %d", args.UpstreamID)
	return nil
}

// PeriodicTokenCheckJobArgs represents arguments for periodic token check
type PeriodicTokenCheckJobArgs struct{}

// Kind returns the unique identifier for this job type
func (PeriodicTokenCheckJobArgs) Kind() string {
	return "periodic_token_check"
}

// PeriodicTokenCheckWorker checks all upstreams and refreshes tokens that need it
type PeriodicTokenCheckWorker struct {
	river.WorkerDefaults[PeriodicTokenCheckJobArgs]
	manager     *upstreams.Manager
	riverClient *river.Client[pgx.Tx]
}

// NewPeriodicTokenCheckWorker creates a new periodic token check worker
func NewPeriodicTokenCheckWorker(manager *upstreams.Manager, riverClient *river.Client[pgx.Tx]) *PeriodicTokenCheckWorker {
	return &PeriodicTokenCheckWorker{
		manager:     manager,
		riverClient: riverClient,
	}
}

// Work processes the periodic token check job
func (w *PeriodicTokenCheckWorker) Work(ctx context.Context, job *river.Job[PeriodicTokenCheckJobArgs]) error {
	log.Println("[River] Starting periodic upstream token check...")

	// Get all upstreams
	upstreams, err := w.manager.ListUpstreams(ctx)
	if err != nil {
		log.Printf("[River] Failed to list upstreams: %v", err)
		return err
	}

	refreshed := 0
	for _, upstream := range upstreams {
		// Skip disabled upstreams
		if !upstream.Enabled {
			continue
		}

		// Check if token needs refresh based on expiry
		// Refresh if token expires within the next 30 minutes
		if time.Now().Add(30 * time.Minute).After(upstream.TokenExpiry) {
			log.Printf("[River] Upstream '%s' (ID: %d) token expires at %s - enqueueing refresh",
				upstream.Name, upstream.ID, upstream.TokenExpiry.Format(time.RFC3339))

			// Enqueue token refresh job
			_, err := w.riverClient.Insert(ctx, TokenRefreshJobArgs{
				UpstreamID: upstream.ID,
			}, &river.InsertOpts{
				Queue: "periodic",
			})
			if err != nil {
				log.Printf("[River] Failed to enqueue token refresh for upstream %d: %v", upstream.ID, err)
				continue
			}
			refreshed++
		}
	}

	log.Printf("[River] Periodic token check complete - enqueued %d token refreshes", refreshed)
	return nil
}
