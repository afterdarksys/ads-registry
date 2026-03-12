package scanner

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/ryan/ads-registry/internal/queue"
)

// RiverWorker wraps River for vulnerability scanning
type RiverWorker struct {
	queueClient *queue.Client
	riverClient *river.Client[pgx.Tx]
}

// NewRiverWorker creates a new River-backed scanner worker
func NewRiverWorker(queueClient *queue.Client) *RiverWorker {
	return &RiverWorker{
		queueClient: queueClient,
		riverClient: queueClient.GetRiverClient(),
	}
}

// Start starts the River client (workers already registered during client creation)
func (w *RiverWorker) Start(ctx context.Context) error {
	// Start the River client
	return w.queueClient.Start(ctx)
}

// Stop gracefully stops the River worker
func (w *RiverWorker) Stop(ctx context.Context) error {
	return w.queueClient.Stop(ctx)
}

// Enqueue adds a vulnerability scan job to the River queue
func (w *RiverWorker) Enqueue(namespace, repo, ref, digest string) {
	_, err := w.riverClient.Insert(context.Background(), queue.ScanJobArgs{
		Namespace: namespace,
		Repo:      repo,
		Reference: ref,
		Digest:    digest,
	}, &river.InsertOpts{
		Queue: "vulnerability",
	})
	if err != nil {
		log.Printf("[River] Failed to enqueue scan for %s/%s:%s (%s): %v",
			namespace, repo, ref, digest, err)
	} else {
		log.Printf("[River] Queued scan for %s/%s:%s (%s)",
			namespace, repo, ref, digest)
	}
}
