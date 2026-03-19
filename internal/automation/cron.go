package automation

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// CronScheduler manages scheduled Starlark script execution
type CronScheduler struct {
	cron         *cron.Cron
	engine       *Engine
	scriptsDir   string
	jobs         map[string]cron.EntryID
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// CronJob represents a scheduled Starlark script
type CronJob struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"` // Cron expression
	Script   string `json:"script"`   // Path to .star file
	Enabled  bool   `json:"enabled"`
	LastRun  time.Time `json:"last_run"`
	NextRun  time.Time `json:"next_run"`
}

// NewCronScheduler creates a new cron scheduler for Starlark scripts
func NewCronScheduler(engine *Engine, scriptsDir string) *CronScheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &CronScheduler{
		cron:       cron.New(),
		engine:     engine,
		scriptsDir: scriptsDir,
		jobs:       make(map[string]cron.EntryID),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start begins the cron scheduler
func (s *CronScheduler) Start() {
	s.cron.Start()
	log.Println("[CRON] Scheduler started")

	// Auto-load cron jobs from scripts/cron/ directory
	if err := s.LoadCronJobs(); err != nil {
		log.Printf("[CRON] Warning: failed to load cron jobs: %v", err)
	}
}

// Stop halts the cron scheduler
func (s *CronScheduler) Stop() {
	s.cancel()
	s.cron.Stop()
	log.Println("[CRON] Scheduler stopped")
}

// AddJob adds a new cron job
// schedule: Cron expression (e.g., "0 */6 * * *" for every 6 hours)
// script: Path to Starlark script
func (s *CronScheduler) AddJob(name, schedule, scriptPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing job with same name if it exists
	if existingID, exists := s.jobs[name]; exists {
		s.cron.Remove(existingID)
		delete(s.jobs, name)
		log.Printf("[CRON] Removed existing job: %s", name)
	}

	// Add new job
	entryID, err := s.cron.AddFunc(schedule, func() {
		s.runScript(name, scriptPath)
	})

	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	s.jobs[name] = entryID
	log.Printf("[CRON] Added job: %s (schedule: %s, script: %s)", name, schedule, scriptPath)

	return nil
}

// RemoveJob removes a cron job by name
func (s *CronScheduler) RemoveJob(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entryID, exists := s.jobs[name]
	if !exists {
		return fmt.Errorf("job not found: %s", name)
	}

	s.cron.Remove(entryID)
	delete(s.jobs, name)

	log.Printf("[CRON] Removed job: %s", name)
	return nil
}

// ListJobs returns all registered cron jobs
func (s *CronScheduler) ListJobs() []CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]CronJob, 0, len(s.jobs))

	for name, entryID := range s.jobs {
		entry := s.cron.Entry(entryID)

		job := CronJob{
			Name:    name,
			Enabled: true,
			NextRun: entry.Next,
		}

		// Try to find the schedule from the entry
		// Note: cron library doesn't expose the schedule string directly
		// We'd need to store it separately for full info

		jobs = append(jobs, job)
	}

	return jobs
}

// LoadCronJobs automatically loads cron jobs from scripts/cron/ directory
// Expected format: {name}.cron.star with metadata in comments
//
// Example script header:
//   # cron: 0 */6 * * *
//   # description: Refresh upstream registry tokens every 6 hours
func (s *CronScheduler) LoadCronJobs() error {
	cronDir := filepath.Join(s.scriptsDir, "cron")

	// Check if cron directory exists
	if _, err := os.Stat(cronDir); os.IsNotExist(err) {
		log.Printf("[CRON] Cron directory not found, creating: %s", cronDir)
		if err := os.MkdirAll(cronDir, 0755); err != nil {
			return fmt.Errorf("failed to create cron directory: %w", err)
		}
		return nil
	}

	files, err := os.ReadDir(cronDir)
	if err != nil {
		return fmt.Errorf("failed to read cron directory: %w", err)
	}

	loaded := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".star") {
			continue
		}

		scriptPath := filepath.Join(cronDir, file.Name())

		// Read script to extract cron schedule from header
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			log.Printf("[CRON] Warning: failed to read %s: %v", file.Name(), err)
			continue
		}

		// Parse cron schedule from comment
		schedule := extractCronSchedule(string(content))
		if schedule == "" {
			log.Printf("[CRON] Warning: no cron schedule found in %s, skipping", file.Name())
			continue
		}

		// Use filename (without .star) as job name
		jobName := strings.TrimSuffix(file.Name(), ".star")

		// Add the job
		if err := s.AddJob(jobName, schedule, scriptPath); err != nil {
			log.Printf("[CRON] Warning: failed to add job %s: %v", jobName, err)
			continue
		}

		loaded++
	}

	log.Printf("[CRON] Loaded %d cron jobs from %s", loaded, cronDir)
	return nil
}

// runScript executes a Starlark script
func (s *CronScheduler) runScript(name, scriptPath string) {
	log.Printf("[CRON] Running job: %s (script: %s)", name, scriptPath)

	startTime := time.Now()

	// Execute script with empty event (cron jobs don't have events)
	err := s.engine.ExecuteEvent(scriptPath, "scheduled", map[string]string{
		"job_name":   name,
		"trigger":    "cron",
		"started_at": startTime.Format(time.RFC3339),
	})

	duration := time.Since(startTime)

	if err != nil {
		log.Printf("[CRON] Job %s failed after %v: %v", name, duration, err)
	} else {
		log.Printf("[CRON] Job %s completed successfully in %v", name, duration)
	}
}

// extractCronSchedule extracts cron schedule from script comments
// Expected format: # cron: 0 */6 * * *
func extractCronSchedule(content string) string {
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for: # cron: <schedule>
		if strings.HasPrefix(line, "#") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(strings.TrimPrefix(parts[0], "#"))
				if key == "cron" {
					return strings.TrimSpace(parts[1])
				}
			}
		}

		// Stop at first non-comment line
		if !strings.HasPrefix(line, "#") && line != "" {
			break
		}
	}

	return ""
}
