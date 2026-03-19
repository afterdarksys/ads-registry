package sync

import (
	"context"
	"log"
	"sync"

	"github.com/ryan/ads-registry/internal/automation"
	"github.com/ryan/ads-registry/internal/config"
)

// Manager handles background synchronization of images to peer registries
type Manager struct {
	peers    []config.PeerRegistry
	starlark *automation.Engine
	jobQueue chan SyncJob
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

type SyncJob struct {
	Namespace  string
	Repository string
	Reference  string
	Digest     string
}

func NewManager(cfg []config.PeerRegistry, starlarkEngine *automation.Engine) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		peers:    cfg,
		starlark: starlarkEngine,
		jobQueue: make(chan SyncJob, 1000), // Buffer for high throughput pushes
		ctx:      ctx,
		cancel:   cancel,
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

		// Perform the sync (placeholder for actual API pushing)
		log.Printf("[SyncManager] Authorized sync pushing %s:%s to %s", job.Repository, job.Reference, peer.Name)
		// ... sync engine goes here ... 
	}
}
