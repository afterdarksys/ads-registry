package npm

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

	r.Get("/{package}", h.getPackageMetadata)
	r.Get("/{package}/-/{tarball}", h.downloadTarball)
	r.Put("/{package}", h.publishPackage)
	
	// Scoped packages
	r.Get("/@{scope}/{package}", h.getScopedPackageMetadata)
	r.Get("/@{scope}/{package}/-/{tarball}", h.downloadScopedTarball)
	r.Put("/@{scope}/{package}", h.publishScopedPackage)

	return r
}

func (h *Handler) getPackageMetadata(w http.ResponseWriter, r *http.Request) {
	pkgName := chi.URLParam(r, "package")
	h.handleGetMetadata(w, r, "default", pkgName)
}

func (h *Handler) getScopedPackageMetadata(w http.ResponseWriter, r *http.Request) {
	scope := chi.URLParam(r, "scope")
	pkgName := chi.URLParam(r, "package")
	fullPkg := fmt.Sprintf("@%s/%s", scope, pkgName)
	h.handleGetMetadata(w, r, "default", fullPkg)
}

func (h *Handler) handleGetMetadata(w http.ResponseWriter, r *http.Request, namespace, pkgName string) {
	ctx := r.Context()

	// Load all versions for this package
	artifacts, err := h.db.ListArtifacts(ctx, "npm", namespace, pkgName)
	if err != nil {
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	if len(artifacts) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not_found", "reason": "document not found"}`))
		return
	}

	// NPM metadata structure reconstruction
	doc := map[string]interface{}{
		"_id":         pkgName,
		"name":        pkgName,
		"description": "",
		"versions":    map[string]interface{}{},
		"dist-tags":   map[string]interface{}{},
		"time":        map[string]interface{}{},
	}

	versions := doc["versions"].(map[string]interface{})
	timeMap := doc["time"].(map[string]interface{})

	var latestVersion string
	for _, artifact := range artifacts {
		var meta map[string]interface{}
		if len(artifact.Metadata) > 0 {
			if err := json.Unmarshal(artifact.Metadata, &meta); err == nil {
				// Modify the dist.tarball URL to point to our registry
				if dist, ok := meta["dist"].(map[string]interface{}); ok {
					protocol := "http"
					if r.TLS != nil {
						protocol = "https"
					}
					// e.g. http://localhost:5005/repository/npm/mypackage/-/mypackage-1.0.0.tgz
					dist["tarball"] = fmt.Sprintf("%s://%s/repository/npm/%s/-/%s-%s.tgz", 
						protocol, r.Host, pkgName, strings.ReplaceAll(pkgName, "/", "%2f"), artifact.Version)
				}
				versions[artifact.Version] = meta
			}
		}
		timeMap[artifact.Version] = artifact.CreatedAt.Format("2006-01-02T15:04:05Z")
		if latestVersion == "" {
			latestVersion = artifact.Version
		}
	}

	// Assign latest tag
	if latestVersion != "" {
		doc["dist-tags"].(map[string]interface{})["latest"] = latestVersion
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (h *Handler) downloadTarball(w http.ResponseWriter, r *http.Request) {
	pkgName := chi.URLParam(r, "package")
	tarball := chi.URLParam(r, "tarball")
	h.handleDownload(w, r, "default", pkgName, tarball)
}

func (h *Handler) downloadScopedTarball(w http.ResponseWriter, r *http.Request) {
	scope := chi.URLParam(r, "scope")
	pkgName := chi.URLParam(r, "package")
	tarball := chi.URLParam(r, "tarball")
	fullPkg := fmt.Sprintf("@%s/%s", scope, pkgName)
	h.handleDownload(w, r, "default", fullPkg, tarball)
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request, namespace, pkgName, tarball string) {
	// The digest is uniquely stored in OCI blob storage, but we need to derive it.
	// For NPM, the Blob digest might just be `sha512-<hex>` or we look it up via ArtifactBlobs.
	// We'll search by exact filename in the storage path layout: /npm/<namespace>/<pkgName>/<tarball>
	
	path := filepath.Join("npm", namespace, pkgName, tarball)
	reader, err := h.storage.Reader(r.Context(), path, 0)
	if err != nil {
		if err == storage.ErrNotFound {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "not found"}`))
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, reader)
}

func (h *Handler) publishPackage(w http.ResponseWriter, r *http.Request) {
	pkgName := chi.URLParam(r, "package")
	h.handlePublish(w, r, "default", pkgName)
}

func (h *Handler) publishScopedPackage(w http.ResponseWriter, r *http.Request) {
	scope := chi.URLParam(r, "scope")
	pkgName := chi.URLParam(r, "package")
	fullPkg := fmt.Sprintf("@%s/%s", scope, pkgName)
	h.handlePublish(w, r, "default", fullPkg)
}

func (h *Handler) handlePublish(w http.ResponseWriter, r *http.Request, namespace, pkgName string) {
	ctx := r.Context()

	// Parse the massive JSON body
	var body struct {
		ID          string                 `json:"_id"`
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		DistTags    map[string]string      `json:"dist-tags"`
		Versions    map[string]interface{} `json:"versions"`
		Attachments map[string]struct {
			ContentType string `json:"content_type"`
			Data        string `json:"data"`
			Length      int    `json:"length"`
		} `json:"_attachments"`
	}

	// Limit reader to avoid memory exhaustion (e.g., 50MB max)
	limitedReader := io.LimitReader(r.Body, 50*1024*1024)
	if err := json.NewDecoder(limitedReader).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "bad request, invalid json"}`))
		return
	}

	// Process each version declared
	for version, versionData := range body.Versions {
		// Convert versionData back to JSON for storage
		versionMetadata, err := json.Marshal(versionData)
		if err != nil {
			continue
		}

		// Create artifact record
		artifact := &db.UniversalArtifact{
			Format:      "npm",
			Namespace:   namespace,
			PackageName: pkgName,
			Version:     version,
			Metadata:    versionMetadata,
		}

		artifactID, err := h.db.CreateArtifact(ctx, artifact)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Process attachments/tarballs
		for filename, attachment := range body.Attachments {
			// Extract tarball from Base64
			decoded, err := base64.StdEncoding.DecodeString(attachment.Data)
			if err != nil {
				continue
			}

			// Calculate Hashes
			shasum := sha1.Sum(decoded)
			sha1Hex := hex.EncodeToString(shasum[:])
			
			sha512sum := sha512.Sum512(decoded)
			sha512Hex := hex.EncodeToString(sha512sum[:])
			digest := fmt.Sprintf("sha512-%s", sha512Hex)

			// Store it directly into OCI Blob storage utilizing the path
			path := filepath.Join("npm", namespace, pkgName, filename)
			writer, err := h.storage.Writer(ctx, path)
			if err != nil {
				continue
			}
			_, _ = io.Copy(writer, bytes.NewReader(decoded))
			writer.Close()

			// Link the blob to the artifact
			_ = h.db.AttachBlob(ctx, artifactID, sha1Hex, filename)
			_ = h.db.AttachBlob(ctx, artifactID, digest, filename)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok": "created", "success": true}`))
}
