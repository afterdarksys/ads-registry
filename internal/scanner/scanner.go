package scanner

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/events"
	"github.com/ryan/ads-registry/internal/storage"
	"github.com/ryan/ads-registry/internal/webhooks"
)

type Engine interface {
	Name() string
	Scan(ctx context.Context, namespace, repo, digest string) (*Report, error)
}

// Scanner interface for job enqueuing (implemented by both Worker and RiverWorker)
type Scanner interface {
	Enqueue(namespace, repo, ref, digest string)
}

// Report represents a standardized vulnerability output
type Report struct {
	Digest          string    `json:"digest"`
	ScannerName     string    `json:"scanner_name"`
	ScannerVersion  string    `json:"scanner_version"`
	CreatedAt       time.Time `json:"created_at"`
	Vulnerabilities []Vuln    `json:"vulnerabilities"`
}

type Vuln struct {
	ID          string `json:"id"`
	Package     string `json:"package"`
	Version     string `json:"version"`
	FixVersion  string `json:"fix_version"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type Worker struct {
	db       db.Store
	storage  storage.Provider
	engines  []Engine
	webhookD *webhooks.Dispatcher
	broker   *events.Broker
	jobs     chan ScanJob
}

type ScanJob struct {
	Namespace string
	Repo      string
	Reference string
	Digest    string
}

func NewWorker(dbStore db.Store, sp storage.Provider, engines []Engine, wd *webhooks.Dispatcher, broker *events.Broker) *Worker {
	return &Worker{
		db:       dbStore,
		storage:  sp,
		engines:  engines,
		webhookD: wd,
		broker:   broker,
		jobs:     make(chan ScanJob, 100), // Buffer
	}
}

func (w *Worker) Start(ctx context.Context, workers int) {
	for i := 0; i < workers; i++ {
		go w.processJobs(ctx)
	}

	// Start the background vulnerability re-scan scheduler
	go w.startCronScheduler(ctx)
}

func (w *Worker) Enqueue(namespace, repo, ref, digest string) {
	select {
	case w.jobs <- ScanJob{Namespace: namespace, Repo: repo, Reference: ref, Digest: digest}:
		log.Printf("Queued scan for %s/%s:%s (%s)", namespace, repo, ref, digest)
	default:
		log.Printf("Warning: Scan queue full, dropping job for %s", digest)
	}
}

func (w *Worker) processJobs(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-w.jobs:
			w.runScans(ctx, job)
		}
	}
}

func (w *Worker) startCronScheduler(ctx context.Context) {
	// For production this would be configurable, default to 24h
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runPeriodicScan(ctx)
		}
	}
}

func (w *Worker) runPeriodicScan(ctx context.Context) {
	log.Println("[Scanner] Starting periodic vulnerability re-scan...")

	// 1. Fetch all known manifests
	records, err := w.db.ListManifests(ctx)
	if err != nil {
		log.Printf("[Scanner] Failed to fetch manifests for re-scan: %v", err)
		return
	}

	// 2. Enqueue them for background scanning
	for _, rec := range records {
		// Optimization: Check if it's already in the queue, or if the report is very fresh (skipped for MVP)
		w.Enqueue(rec.Namespace, rec.Repo, rec.Reference, rec.Digest)
	}
}

func (w *Worker) runScans(ctx context.Context, job ScanJob) {
	// 1. Notify external 3rd party webhooks that a new image is ready
	// This allows products like Snyk/Datadog to pull the image and scan
	w.webhookD.Dispatch(ctx, "image.pushed", map[string]string{
		"namespace": job.Namespace,
		"repo":      job.Repo,
		"tag":       job.Reference,
		"digest":    job.Digest,
	})

	// 2. Run local embedded scanners (e.g., Trivy wrapper)
	for _, engine := range w.engines {
		report, err := engine.Scan(ctx, job.Namespace, job.Repo, job.Digest)
		if err != nil {
			log.Printf("Scanner [%T] failed on %s: %v", engine, job.Digest, err)
			continue
		}

		// 3. Save report to DB
		data, _ := json.Marshal(report)
		log.Printf("Saved Vulnerability Report for %s: %s", job.Digest, string(data))

		err = w.db.SaveScanReport(ctx, job.Digest, engine.Name(), data)
		if err != nil {
			log.Printf("Failed to save vulnerability report to DB for %s: %v", job.Digest, err)
			continue
		}
		if w.broker != nil {
			w.broker.Publish("scan.complete", map[string]string{
				"namespace": job.Namespace,
				"repo":      job.Repo,
				"digest":    job.Digest,
				"scanner":   engine.Name(),
			})
		}
	}
}
