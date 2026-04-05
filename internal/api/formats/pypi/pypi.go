package pypi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strings"

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

	// Simple Repository API (PEP 503)
	r.Get("/simple/", h.listPackages)
	r.Get("/simple/{package}/", h.packageDetails)

	// API Upload (twine)
	r.Post("/", h.uploadPackage)

	// Package Download (e.g. /packages/1.0.0/my_pkg-1.0.0-py3-none-any.whl)
	r.Get("/packages/{version}/{filename}", h.downloadPackage)

	return r
}

func (h *Handler) listPackages(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	
	// Since we don't have a specific ListUniquePackageNames in ArtifactStore yet,
	// we will run a dummy search query to get all PyPI artifacts and extract unique names.
	artifacts, err := h.db.SearchArtifacts(r.Context(), "pypi", namespace, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	uniqueNames := make(map[string]bool)
	for _, a := range artifacts {
		uniqueNames[a.PackageName] = true
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!DOCTYPE html>\n<html>\n  <head>\n    <title>Simple Index</title>\n  </head>\n  <body>\n")
	for name := range uniqueNames {
		fmt.Fprintf(w, "    <a href=\"%s/\">%s</a><br/>\n", template.HTMLEscapeString(name), template.HTMLEscapeString(name))
	}
	fmt.Fprintf(w, "  </body>\n</html>\n")
}

func (h *Handler) packageDetails(w http.ResponseWriter, r *http.Request) {
	pkgName := chi.URLParam(r, "package")
	namespace := "default"

	artifacts, err := h.db.ListArtifacts(r.Context(), "pypi", namespace, pkgName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(artifacts) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!DOCTYPE html>\n<html>\n  <head>\n    <title>Links for %s</title>\n  </head>\n  <body>\n    <h1>Links for %s</h1>\n", template.HTMLEscapeString(pkgName), template.HTMLEscapeString(pkgName))

	for _, artifact := range artifacts {
		// Expecting blobs to contain the wheel/sdist and its SHA256 digest
		for _, blob := range artifact.Blobs {
			// Build URL back to the registry
			protocol := "http"
			if r.TLS != nil {
				protocol = "https"
			}
			link := fmt.Sprintf("%s://%s/repository/pypi/packages/%s/%s#sha256=%s", protocol, r.Host, artifact.Version, blob.FileName, blob.BlobDigest)
			fmt.Fprintf(w, "    <a href=\"%s\">%s</a><br/>\n", link, template.HTMLEscapeString(blob.FileName))
		}
	}
	fmt.Fprintf(w, "  </body>\n</html>\n")
}

func (h *Handler) uploadPackage(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	
	err := r.ParseMultipartForm(50 << 20) // 50MB max memory
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	pkgName := r.FormValue("name")
	version := r.FormValue("version")
	if pkgName == "" || version == "" {
		http.Error(w, "Name and version required", http.StatusBadRequest)
		return
	}

	// Normalize package name to standard PyPI format (lowercase, hyphens for underscores)
	pkgName = strings.ToLower(strings.ReplaceAll(pkgName, "_", "-"))

	file, fileHeader, err := r.FormFile("content")
	if err != nil {
		http.Error(w, "File upload required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Extract standard PyPI metadata from twine
	metadata := map[string]interface{}{
		"summary":          r.FormValue("summary"),
		"author":           r.FormValue("author"),
		"author_email":     r.FormValue("author_email"),
		"home_page":        r.FormValue("home_page"),
		"metadata_version": r.FormValue("metadata_version"),
	}
	metaBytes, _ := json.Marshal(metadata)

	artifact := &db.UniversalArtifact{
		Format:      "pypi",
		Namespace:   namespace,
		PackageName: pkgName,
		Version:     version,
		Metadata:    metaBytes,
	}

	artifactID, err := h.db.CreateArtifact(r.Context(), artifact)
	if err != nil {
		http.Error(w, "Failed to create artifact", http.StatusInternalServerError)
		return
	}

	// Write file directly to OCI blobs
	hasher := sha256.New()
	path := filepath.Join("pypi", namespace, pkgName, version, fileHeader.Filename)
	writer, err := h.storage.Writer(r.Context(), path)
	if err != nil {
		http.Error(w, "Failed to initialize storage", http.StatusInternalServerError)
		return
	}
	
	// TeeReader calculates hash while streaming to storage
	tee := io.TeeReader(file, hasher)
	if _, err := io.Copy(writer, tee); err != nil {
		writer.Close()
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	writer.Close()

	sha256Hex := hex.EncodeToString(hasher.Sum(nil))

	// Link Blob to artifact
	if err := h.db.AttachBlob(r.Context(), artifactID, sha256Hex, fileHeader.Filename); err != nil {
		http.Error(w, "Failed to attach blob", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Upload successful"))
}

func (h *Handler) downloadPackage(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	version := chi.URLParam(r, "version")
	filename := chi.URLParam(r, "filename")

	// Derive the package name from filename.
	// We can cheat slightly since wheel/sdist filenames begin with the package name.
	// But it's easier to find it dynamically or just attempt to load by standard path construction:
	// A wheel usually looks like `package_name-1.0.0-py3-none-any.whl`
	parts := strings.Split(filename, "-")
	
	// If it's an sdist ending in .tar.gz, splitting on - works differently, but we can reconstruct:
	// Usually if it's `pkg-1.0.0.tar.gz`, splitting by `-` gives `pkg`, `1.0.0.tar.gz`.
	// For PyPI path construction, let's normalize:
	var pkgName string
	if len(parts) > 1 {
		pkgName = parts[0]
	} else {
		pkgName = filename // Fallback
	}
	pkgName = strings.ToLower(strings.ReplaceAll(pkgName, "_", "-"))

	path := filepath.Join("pypi", namespace, pkgName, version, filename)
	
	reader, err := h.storage.Reader(r.Context(), path, 0)
	if err != nil {
		if err == storage.ErrNotFound {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, reader)
}
