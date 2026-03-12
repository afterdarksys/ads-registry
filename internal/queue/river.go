package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
	"github.com/ryan/ads-registry/internal/upstreams"
	"github.com/ryan/ads-registry/internal/webhooks"
)

// Client wraps River's client for job queue operations
type Client struct {
	riverClient *river.Client[pgx.Tx]
	pool        *pgxpool.Pool
}

// NewClient creates a new River client from a PostgreSQL connection string
func NewClient(ctx context.Context, dsn string, defaultWorkers, vulnWorkers, periodicWorkers int,
	dbStore db.Store, sp storage.Provider, engines []ScanEngine, wd *webhooks.Dispatcher,
	upstreamMgr *upstreams.Manager) (*Client, error) {
	// Create pgxpool connection
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	workers := river.NewWorkers()

	// Create River client with workers configuration
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: defaultWorkers},
			"vulnerability":    {MaxWorkers: vulnWorkers},
			"periodic":         {MaxWorkers: periodicWorkers},
		},
		Workers: workers,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to create river client: %w", err)
	}

	// Register workers
	scanWorker := NewScanJobWorker(dbStore, sp, engines, wd, nil) // TODO: Wire up notification service
	periodicWorker := NewPeriodicRescanWorker(dbStore, riverClient)

	river.AddWorker(workers, scanWorker)
	river.AddWorker(workers, periodicWorker)

	// Register upstream token refresh workers (if upstreamMgr provided)
	if upstreamMgr != nil {
		tokenRefreshWorker := NewTokenRefreshWorker(upstreamMgr)
		periodicTokenCheckWorker := NewPeriodicTokenCheckWorker(upstreamMgr, riverClient)

		river.AddWorker(workers, tokenRefreshWorker)
		river.AddWorker(workers, periodicTokenCheckWorker)
	}

	return &Client{
		riverClient: riverClient,
		pool:        pool,
	}, nil
}

// Start begins processing jobs
func (c *Client) Start(ctx context.Context) error {
	return c.riverClient.Start(ctx)
}

// Stop gracefully stops the River client
func (c *Client) Stop(ctx context.Context) error {
	return c.riverClient.Stop(ctx)
}

// Close closes the underlying database connection
func (c *Client) Close() error {
	c.pool.Close()
	return nil
}

// GetRiverClient returns the underlying River client for job insertion
func (c *Client) GetRiverClient() *river.Client[pgx.Tx] {
	return c.riverClient
}
