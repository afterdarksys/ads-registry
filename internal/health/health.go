package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status represents the health status of a component
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// Check represents a health check result
type Check struct {
	Name      string        `json:"name"`
	Status    Status        `json:"status"`
	Message   string        `json:"message,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration_ms"`
}

// Response represents the overall health response
type Response struct {
	Status    Status           `json:"status"`
	Timestamp time.Time        `json:"timestamp"`
	Version   string           `json:"version,omitempty"`
	Checks    map[string]Check `json:"checks"`
}

// Checker defines the interface for health checks
type Checker interface {
	Check(ctx context.Context) Check
}

// CheckerFunc is a function that implements Checker
type CheckerFunc func(ctx context.Context) Check

func (f CheckerFunc) Check(ctx context.Context) Check {
	return f(ctx)
}

// Handler manages health check endpoints
type Handler struct {
	mu             sync.RWMutex
	startupChecks  map[string]Checker
	readyChecks    map[string]Checker
	livenessChecks map[string]Checker
	version        string
	startedAt      time.Time
}

// NewHandler creates a new health check handler
func NewHandler(version string) *Handler {
	return &Handler{
		startupChecks:  make(map[string]Checker),
		readyChecks:    make(map[string]Checker),
		livenessChecks: make(map[string]Checker),
		version:        version,
		startedAt:      time.Now(),
	}
}

// RegisterStartup adds a startup probe check
func (h *Handler) RegisterStartup(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.startupChecks[name] = checker
}

// RegisterReadiness adds a readiness probe check
func (h *Handler) RegisterReadiness(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readyChecks[name] = checker
}

// RegisterLiveness adds a liveness probe check
func (h *Handler) RegisterLiveness(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.livenessChecks[name] = checker
}

// Startup handles startup probe requests
// Kubernetes uses this to know when the application has started
func (h *Handler) Startup(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	checks := h.startupChecks
	h.mu.RUnlock()

	response := h.runChecks(r.Context(), checks)
	h.writeResponse(w, response)
}

// Readiness handles readiness probe requests
// Kubernetes uses this to know if the app can accept traffic
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	checks := h.readyChecks
	h.mu.RUnlock()

	response := h.runChecks(r.Context(), checks)
	h.writeResponse(w, response)
}

// Liveness handles liveness probe requests
// Kubernetes uses this to know if the app should be restarted
func (h *Handler) Liveness(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	checks := h.livenessChecks
	h.mu.RUnlock()

	response := h.runChecks(r.Context(), checks)
	h.writeResponse(w, response)
}

// Health handles general health check requests
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	// Combine all checks for general health endpoint
	allChecks := make(map[string]Checker)
	for k, v := range h.readyChecks {
		allChecks[k] = v
	}
	for k, v := range h.livenessChecks {
		if _, exists := allChecks[k]; !exists {
			allChecks[k] = v
		}
	}
	h.mu.RUnlock()

	response := h.runChecks(r.Context(), allChecks)
	h.writeResponse(w, response)
}

func (h *Handler) runChecks(ctx context.Context, checks map[string]Checker) Response {
	response := Response{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Version:   h.version,
		Checks:    make(map[string]Check),
	}

	// Run all checks in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, checker := range checks {
		wg.Add(1)
		go func(name string, checker Checker) {
			defer wg.Done()

			check := checker.Check(ctx)

			mu.Lock()
			response.Checks[name] = check

			// Downgrade overall status if any check is unhealthy
			if check.Status == StatusUnhealthy && response.Status != StatusUnhealthy {
				response.Status = StatusUnhealthy
			} else if check.Status == StatusDegraded && response.Status == StatusHealthy {
				response.Status = StatusDegraded
			}
			mu.Unlock()
		}(name, checker)
	}

	wg.Wait()

	return response
}

func (h *Handler) writeResponse(w http.ResponseWriter, response Response) {
	w.Header().Set("Content-Type", "application/json")

	// Set HTTP status based on health
	switch response.Status {
	case StatusHealthy:
		w.WriteHeader(http.StatusOK)
	case StatusDegraded:
		w.WriteHeader(http.StatusOK) // Still 200, but degraded
	case StatusUnhealthy:
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}

// DatabaseChecker checks database connectivity
func DatabaseChecker(db *sql.DB, timeout time.Duration) Checker {
	return CheckerFunc(func(ctx context.Context) Check {
		start := time.Now()
		check := Check{
			Name:      "database",
			Timestamp: start,
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			check.Status = StatusUnhealthy
			check.Message = "database ping failed: " + err.Error()
		} else {
			check.Status = StatusHealthy
			check.Message = "database connection OK"
		}

		check.Duration = time.Since(start)
		return check
	})
}

// StorageChecker checks if storage is accessible
func StorageChecker(checkFunc func(ctx context.Context) error, timeout time.Duration) Checker {
	return CheckerFunc(func(ctx context.Context) Check {
		start := time.Now()
		check := Check{
			Name:      "storage",
			Timestamp: start,
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		if err := checkFunc(ctx); err != nil {
			check.Status = StatusUnhealthy
			check.Message = "storage check failed: " + err.Error()
		} else {
			check.Status = StatusHealthy
			check.Message = "storage accessible"
		}

		check.Duration = time.Since(start)
		return check
	})
}

// UptimeChecker reports how long the service has been running
func UptimeChecker(startedAt time.Time) Checker {
	return CheckerFunc(func(ctx context.Context) Check {
		uptime := time.Since(startedAt)
		return Check{
			Name:      "uptime",
			Status:    StatusHealthy,
			Message:   uptime.String(),
			Timestamp: time.Now(),
			Duration:  0,
		}
	})
}
