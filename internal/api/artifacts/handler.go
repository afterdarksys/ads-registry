package artifacts

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/db"
)

// Handler manages artifact API endpoints
type Handler struct {
	store db.Store
}

// NewHandler creates a new artifacts API handler
func NewHandler(store db.Store) *Handler {
	return &Handler{store: store}
}

// Router returns the chi router for artifact management
func (h *Handler) Router() chi.Router {
	r := chi.NewRouter()

	// Package listing and info
	r.Get("/{format}/{namespace}", h.listPackages)
	r.Get("/{format}/{namespace}/{package}", h.listVersions)
	r.Get("/{format}/{namespace}/{package}/{version}", h.getArtifact)

	// Package deletion
	r.Delete("/{format}/{namespace}/{package}", h.deleteAllVersions)
	r.Delete("/{format}/{namespace}/{package}/{version}", h.deleteVersion)

	return r
}

// listPackages returns all package names for a format/namespace
func (h *Handler) listPackages(w http.ResponseWriter, r *http.Request) {
	format := chi.URLParam(r, "format")
	namespace := chi.URLParam(r, "namespace")

	names, err := h.store.GetPackageNames(r.Context(), format, namespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get version info for each package
	type PackageInfo struct {
		Name      string   `json:"name"`
		Latest    string   `json:"latest"`
		Versions  []string `json:"versions"`
		CreatedAt string   `json:"created_at"`
	}

	packages := []PackageInfo{}
	for _, name := range names {
		artifacts, err := h.store.ListArtifacts(r.Context(), format, namespace, name)
		if err != nil || len(artifacts) == 0 {
			continue
		}

		versions := make([]string, len(artifacts))
		for i, a := range artifacts {
			versions[i] = a.Version
		}

		packages = append(packages, PackageInfo{
			Name:      name,
			Latest:    artifacts[0].Version,
			Versions:  versions,
			CreatedAt: artifacts[0].CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"packages": packages,
	})
}

// listVersions returns all versions for a package
func (h *Handler) listVersions(w http.ResponseWriter, r *http.Request) {
	format := chi.URLParam(r, "format")
	namespace := chi.URLParam(r, "namespace")
	packageName := chi.URLParam(r, "package")

	artifacts, err := h.store.ListArtifacts(r.Context(), format, namespace, packageName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(artifacts) == 0 {
		http.Error(w, "Package not found", http.StatusNotFound)
		return
	}

	type VersionInfo struct {
		Version   string `json:"version"`
		CreatedAt string `json:"created_at"`
	}

	versions := make([]VersionInfo, len(artifacts))
	for i, a := range artifacts {
		versions[i] = VersionInfo{
			Version:   a.Version,
			CreatedAt: a.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"package":  packageName,
		"versions": versions,
	})
}

// getArtifact returns detailed info about a specific version
func (h *Handler) getArtifact(w http.ResponseWriter, r *http.Request) {
	format := chi.URLParam(r, "format")
	namespace := chi.URLParam(r, "namespace")
	packageName := chi.URLParam(r, "package")
	version := chi.URLParam(r, "version")

	artifact, err := h.store.GetArtifact(r.Context(), format, namespace, packageName, version)
	if err == db.ErrNotFound {
		http.Error(w, "Artifact not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type BlobInfo struct {
		FileName   string `json:"file_name"`
		BlobDigest string `json:"blob_digest"`
		CreatedAt  string `json:"created_at"`
	}

	blobs := make([]BlobInfo, len(artifact.Blobs))
	for i, b := range artifact.Blobs {
		blobs[i] = BlobInfo{
			FileName:   b.FileName,
			BlobDigest: b.BlobDigest,
			CreatedAt:  b.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	response := map[string]interface{}{
		"format":       artifact.Format,
		"namespace":    artifact.Namespace,
		"package_name": artifact.PackageName,
		"version":      artifact.Version,
		"created_at":   artifact.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"blobs":        blobs,
	}

	// Include metadata if present
	if len(artifact.Metadata) > 0 {
		var metadata interface{}
		if err := json.Unmarshal(artifact.Metadata, &metadata); err == nil {
			response["metadata"] = metadata
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// deleteVersion removes a specific version
func (h *Handler) deleteVersion(w http.ResponseWriter, r *http.Request) {
	format := chi.URLParam(r, "format")
	namespace := chi.URLParam(r, "namespace")
	packageName := chi.URLParam(r, "package")
	version := chi.URLParam(r, "version")

	err := h.store.DeleteArtifact(r.Context(), format, namespace, packageName, version)
	if err == db.ErrNotFound {
		http.Error(w, "Artifact not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Artifact deleted successfully",
	})
}

// deleteAllVersions removes all versions of a package
func (h *Handler) deleteAllVersions(w http.ResponseWriter, r *http.Request) {
	format := chi.URLParam(r, "format")
	namespace := chi.URLParam(r, "namespace")
	packageName := chi.URLParam(r, "package")

	err := h.store.DeleteAllArtifactVersions(r.Context(), format, namespace, packageName)
	if err == db.ErrNotFound {
		http.Error(w, "Package not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "All package versions deleted successfully",
	})
}
