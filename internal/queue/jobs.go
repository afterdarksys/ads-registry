package queue

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/events"
	"github.com/ryan/ads-registry/internal/storage"
	"github.com/ryan/ads-registry/internal/webhooks"
)

// ScanEngine interface to avoid import cycle with scanner package
type ScanEngine interface {
	Name() string
	Scan(ctx context.Context, namespace, repo, digest string) (*ScanReport, error)
}

// ScanReport represents a vulnerability scan report
type ScanReport struct {
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

// ScanJobArgs represents the arguments for a vulnerability scan job
type ScanJobArgs struct {
	Namespace string `json:"namespace"`
	Repo      string `json:"repo"`
	Reference string `json:"reference"`
	Digest    string `json:"digest"`
}

// Kind returns the unique identifier for this job type
func (ScanJobArgs) Kind() string {
	return "vulnerability_scan"
}

// ScanJobWorker processes vulnerability scan jobs
type ScanJobWorker struct {
	river.WorkerDefaults[ScanJobArgs]
	db              db.Store
	storage         storage.Provider
	engines         []ScanEngine
	webhookD        *webhooks.Dispatcher
	broker          *events.Broker
	notificationSvc NotificationService
}

// NotificationService interface to avoid import cycles
type NotificationService interface {
	NotifyOwnerOfScanResults(ctx context.Context, digest string, report *ScanReport) error
	SaveScanResultsToDatabase(ctx context.Context, manifestID int, report *ScanReport) error
}

// NewScanJobWorker creates a new vulnerability scan worker
func NewScanJobWorker(dbStore db.Store, sp storage.Provider, engines []ScanEngine, wd *webhooks.Dispatcher, broker *events.Broker, notifSvc NotificationService) *ScanJobWorker {
	return &ScanJobWorker{
		db:              dbStore,
		storage:         sp,
		engines:         engines,
		webhookD:        wd,
		broker:          broker,
		notificationSvc: notifSvc,
	}
}

// Work processes a single vulnerability scan job
func (w *ScanJobWorker) Work(ctx context.Context, job *river.Job[ScanJobArgs]) error {
	args := job.Args

	log.Printf("[River] Processing vulnerability scan for %s/%s:%s (%s)",
		args.Namespace, args.Repo, args.Reference, args.Digest)

	// 1. Notify external 3rd party webhooks that a new image is ready
	w.webhookD.Dispatch(ctx, "image.pushed", map[string]string{
		"namespace": args.Namespace,
		"repo":      args.Repo,
		"tag":       args.Reference,
		"digest":    args.Digest,
	})

	// 2. Run local embedded scanners
	for _, engine := range w.engines {
		report, err := engine.Scan(ctx, args.Namespace, args.Repo, args.Digest)
		if err != nil {
			log.Printf("[River] Scanner [%s] failed on %s: %v", engine.Name(), args.Digest, err)
			continue
		}

		// 3. Save report to DB (legacy table)
		data, _ := json.Marshal(report)
		log.Printf("[River] Saved vulnerability report for %s: %s", args.Digest, string(data))

		err = w.db.SaveScanReport(ctx, args.Digest, engine.Name(), data)
		if err != nil {
			log.Printf("[River] Failed to save vulnerability report to DB for %s: %v", args.Digest, err)
			return err
		}
		if w.broker != nil {
			w.broker.Publish("scan.complete", map[string]string{
				"namespace": args.Namespace,
				"repo":      args.Repo,
				"digest":    args.Digest,
				"scanner":   engine.Name(),
			})
		}

		// 4. Send notifications to image owner (if notification service is configured)
		if w.notificationSvc != nil {
			if err := w.notificationSvc.NotifyOwnerOfScanResults(ctx, args.Digest, report); err != nil {
				log.Printf("[River] Failed to send notifications: %v", err)
			} else {
				log.Printf("[River] Sent scan notifications for %s", args.Digest)
			}
		}
	}

	return nil
}

// PeriodicRescanJobArgs represents arguments for the periodic rescan job
type PeriodicRescanJobArgs struct{}

// Kind returns the unique identifier for this job type
func (PeriodicRescanJobArgs) Kind() string {
	return "periodic_rescan"
}

// PeriodicRescanWorker handles periodic re-scanning of all manifests
type PeriodicRescanWorker struct {
	river.WorkerDefaults[PeriodicRescanJobArgs]
	db          db.Store
	riverClient *river.Client[pgx.Tx]
}

// NewPeriodicRescanWorker creates a new periodic rescan worker
func NewPeriodicRescanWorker(dbStore db.Store, riverClient *river.Client[pgx.Tx]) *PeriodicRescanWorker {
	return &PeriodicRescanWorker{
		db:          dbStore,
		riverClient: riverClient,
	}
}

// Work processes the periodic rescan job
func (w *PeriodicRescanWorker) Work(ctx context.Context, job *river.Job[PeriodicRescanJobArgs]) error {
	log.Println("[River] Starting periodic vulnerability re-scan...")

	// Fetch all known manifests
	records, err := w.db.ListManifests(ctx)
	if err != nil {
		log.Printf("[River] Failed to fetch manifests for re-scan: %v", err)
		return err
	}

	// Enqueue vulnerability scans for each manifest
	for _, rec := range records {
		_, err := w.riverClient.Insert(ctx, ScanJobArgs{
			Namespace: rec.Namespace,
			Repo:      rec.Repo,
			Reference: rec.Reference,
			Digest:    rec.Digest,
		}, &river.InsertOpts{
			Queue: "vulnerability",
		})
		if err != nil {
			log.Printf("[River] Failed to enqueue scan for %s: %v", rec.Digest, err)
		}
	}

	log.Printf("[River] Enqueued %d vulnerability scans", len(records))
	return nil
}
