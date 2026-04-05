package golang

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

type Handler struct {
	db      db.Store
	storage storage.Provider
}

func NewHandler(dbStore db.Store, storageProvider storage.Provider) *Handler {
	return &Handler{
		db:      dbStore,
		storage: storageProvider,
	}
}

func (h *Handler) Router() chi.Router {
	r := chi.NewRouter()

	r.Post("/upload", h.uploadModule)
	// Because Go module paths can have arbitrary numbers of slashes (github.com/user/project/subpkg),
	// we will intercept all GET requests and manually parse the GOPROXY path structure.
	r.Get("/*", h.serveProxy)

	return r
}

func (h *Handler) serveProxy(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	
	if strings.HasSuffix(path, "/@v/list") {
		modulePath := strings.TrimSuffix(path, "/@v/list")
		h.listVersions(w, r, modulePath)
		return
	}

	if idx := strings.LastIndex(path, "/@v/"); idx != -1 {
		modulePath := path[:idx]
		filePart := path[idx+4:] // e.g., v1.0.0.info

		if strings.HasSuffix(filePart, ".info") {
			version := strings.TrimSuffix(filePart, ".info")
			h.moduleInfo(w, r, modulePath, version)
			return
		}
		if strings.HasSuffix(filePart, ".mod") {
			version := strings.TrimSuffix(filePart, ".mod")
			h.moduleMod(w, r, modulePath, version)
			return
		}
		if strings.HasSuffix(filePart, ".zip") {
			version := strings.TrimSuffix(filePart, ".zip")
			h.moduleZip(w, r, modulePath, version)
			return
		}
	}

	http.Error(w, "Not found", http.StatusNotFound)
}

func (h *Handler) listVersions(w http.ResponseWriter, r *http.Request, module string) {
	namespace := "default"
	artifacts, err := h.db.ListArtifacts(r.Context(), "go", namespace, module)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, a := range artifacts {
		fmt.Fprintf(w, "%s\n", a.Version)
	}
}

func (h *Handler) moduleInfo(w http.ResponseWriter, r *http.Request, module, version string) {
	namespace := "default"
	artifact, err := h.db.GetArtifact(r.Context(), "go", namespace, module, version)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	info := map[string]string{
		"Version": artifact.Version,
		"Time":    artifact.CreatedAt.Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (h *Handler) moduleMod(w http.ResponseWriter, r *http.Request, module, version string) {
	namespace := "default"
	filename := fmt.Sprintf("%s.mod", version)
	h.downloadGoFile(w, r, namespace, module, version, filename)
}

func (h *Handler) moduleZip(w http.ResponseWriter, r *http.Request, module, version string) {
	namespace := "default"
	filename := fmt.Sprintf("%s.zip", version)
	h.downloadGoFile(w, r, namespace, module, version, filename)
}

func (h *Handler) downloadGoFile(w http.ResponseWriter, r *http.Request, namespace, module, version, filename string) {
	// Storage path: go/{namespace}/{module}/@v/{filename}
	path := filepath.Join("go", namespace, module, "@v", filename)

	reader, err := h.storage.Reader(r.Context(), path, 0)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer reader.Close()

	if strings.HasSuffix(filename, ".mod") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else if strings.HasSuffix(filename, ".zip") {
		w.Header().Set("Content-Type", "application/zip")
	}

	io.Copy(w, reader)
}

func (h *Handler) uploadModule(w http.ResponseWriter, r *http.Request) {
	namespace := "default"

	err := r.ParseMultipartForm(50 << 20) // 50MB
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	module := r.FormValue("module")
	version := r.FormValue("version")
	if module == "" || version == "" {
		http.Error(w, "Module path and version required (e.g., github.com/foo/bar, v1.0.0)", http.StatusBadRequest)
		return
	}

	modFile, _, err := r.FormFile("mod")
	if err != nil {
		http.Error(w, ".mod file required", http.StatusBadRequest)
		return
	}
	defer modFile.Close()

	zipFile, _, err := r.FormFile("zip")
	if err != nil {
		http.Error(w, ".zip file required", http.StatusBadRequest)
		return
	}
	defer zipFile.Close()

	// Read mod bytes
	modData, _ := io.ReadAll(modFile)

	// Save artifact in DB
	artifact := &db.UniversalArtifact{
		Format:      "go",
		Namespace:   namespace,
		PackageName: module,
		Version:     version,
		Metadata:    []byte("{}"),
	}

	artifactID, err := h.db.CreateArtifact(r.Context(), artifact)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Calculate and write .zip file
	hasher := sha256.New()
	zipPath := filepath.Join("go", namespace, module, "@v", fmt.Sprintf("%s.zip", version))
	zipWriter, _ := h.storage.Writer(r.Context(), zipPath)
	
	tee := io.TeeReader(zipFile, hasher)
	_, _ = io.Copy(zipWriter, tee)
	zipWriter.Close()
	zipDigest := hex.EncodeToString(hasher.Sum(nil))

	if err := h.db.AttachBlob(r.Context(), artifactID, zipDigest, fmt.Sprintf("%s.zip", version)); err != nil {
		http.Error(w, "Failed attaching blob", http.StatusInternalServerError)
		return
	}

	// Calculate and write .mod file
	modHasher := sha256.New()
	modHasher.Write(modData)
	modDigest := hex.EncodeToString(modHasher.Sum(nil))

	modPath := filepath.Join("go", namespace, module, "@v", fmt.Sprintf("%s.mod", version))
	modWriter, _ := h.storage.Writer(r.Context(), modPath)
	_, _ = io.Copy(modWriter, bytes.NewReader(modData))
	modWriter.Close()

	h.db.AttachBlob(r.Context(), artifactID, modDigest, fmt.Sprintf("%s.mod", version))

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Go module uploaded successfully"))
}
