package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

// LivenessHandler checks if the process is alive
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "alive",
		})
	}
}

// ReadinessHandler checks if the service can handle traffic
func ReadinessHandler(store db.Store, storageProvider storage.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := make(map[string]string)
		healthy := true

		// Check database connectivity
		if err := pingDatabase(ctx, store); err != nil {
			checks["database"] = "unhealthy: " + err.Error()
			healthy = false
		} else {
			checks["database"] = "healthy"
		}

		// Check storage backend
		if err := checkStorage(ctx, storageProvider); err != nil {
			checks["storage"] = "unhealthy: " + err.Error()
			healthy = false
		} else {
			checks["storage"] = "healthy"
		}

		w.Header().Set("Content-Type", "application/json")

		if healthy {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "ready",
				"checks": checks,
			})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "not ready",
				"checks": checks,
			})
		}
	}
}

func pingDatabase(ctx context.Context, store db.Store) error {
	// Try a simple query
	_, err := store.ListManifests(ctx)
	return err
}

func checkStorage(ctx context.Context, storageProvider storage.Provider) error {
	// Try to write and read a health check file
	testPath := ".healthcheck"

	writer, err := storageProvider.Writer(ctx, testPath)
	if err != nil {
		return err
	}

	if _, err := writer.Write([]byte("ok")); err != nil {
		writer.Close()
		return err
	}
	writer.Close()

	// Try reading it back
	reader, err := storageProvider.Reader(ctx, testPath, 0)
	if err != nil {
		return err
	}
	reader.Close()

	// Clean up
	storageProvider.Delete(ctx, testPath)

	return nil
}
