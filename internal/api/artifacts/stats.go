package artifacts

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetStatistics returns repository statistics
func (h *Handler) GetStatistics(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	namespace := r.URL.Query().Get("namespace")

	if namespace == "" {
		namespace = "default"
	}

	stats, err := h.store.GetArtifactStatistics(r.Context(), format, namespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// RegisterStatsRoute adds the statistics endpoint to a router
func (h *Handler) RegisterStatsRoute(r chi.Router) {
	r.Get("/stats", h.GetStatistics)
}
