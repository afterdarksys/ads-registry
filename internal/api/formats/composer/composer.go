package composer

import (
	"archive/zip"
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

	r.Post("/upload", h.uploadPackage)
	
	r.Get("/packages.json", h.packagesJson)
	r.Get("/p2/{vendor}/{package}.json", h.packageMetadata)
	r.Get("/dists/{vendor}/{package}/{filename}", h.downloadDist)

	return r
}

func (h *Handler) packagesJson(w http.ResponseWriter, r *http.Request) {
	protocol := "http"
	if r.TLS != nil {
		protocol = "https"
	}
	baseURL := fmt.Sprintf("%s://%s/repository/composer", protocol, r.Host)

	doc := map[string]interface{}{
		"packages": []interface{}{},
		"metadata-url": fmt.Sprintf("%s/p2/%%package%%.json", baseURL),
		"providers-url": fmt.Sprintf("%s/p2/%%package%%.json", baseURL),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (h *Handler) packageMetadata(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	vendor := chi.URLParam(r, "vendor")
	pkg := chi.URLParam(r, "package")
	fullPkgName := fmt.Sprintf("%s/%s", vendor, pkg)

	artifacts, err := h.db.ListArtifacts(r.Context(), "composer", namespace, fullPkgName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(artifacts) == 0 {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"packages":{}}`))
		return
	}

	protocol := "http"
	if r.TLS != nil {
		protocol = "https"
	}
	baseURL := fmt.Sprintf("%s://%s/repository/composer", protocol, r.Host)

	versions := make(map[string]interface{})
	for _, artifact := range artifacts {
		if len(artifact.Metadata) == 0 {
			continue
		}

		var compJson map[string]interface{}
		if err := json.Unmarshal(artifact.Metadata, &compJson); err != nil {
			continue
		}

		// Inject dist details for downloading
		for _, blob := range artifact.Blobs {
			compJson["dist"] = map[string]interface{}{
				"type": "zip",
				"url":  fmt.Sprintf("%s/dists/%s/%s", baseURL, fullPkgName, blob.FileName),
				"reference": blob.BlobDigest, // we'll use sha256 for reference
				"shasum": blob.BlobDigest,
			}
			break
		}

		// Ensure version is set exactly as DB
		compJson["version"] = artifact.Version
		compJson["time"] = artifact.CreatedAt.Format(time.RFC3339)
		versions[artifact.Version] = compJson
	}

	out := map[string]interface{}{
		"packages": map[string]interface{}{
			fullPkgName: versions,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *Handler) downloadDist(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	vendor := chi.URLParam(r, "vendor")
	pkg := chi.URLParam(r, "package")
	filename := chi.URLParam(r, "filename")

	fullPkgName := fmt.Sprintf("%s/%s", vendor, pkg)
	path := filepath.Join("composer", namespace, fullPkgName, filename)

	reader, err := h.storage.Reader(r.Context(), path, 0)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/zip")
	io.Copy(w, reader)
}

func (h *Handler) uploadPackage(w http.ResponseWriter, r *http.Request) {
	namespace := "default"

	err := r.ParseMultipartForm(100 << 20) // 100MB
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("package")
	if err != nil {
		http.Error(w, "ZIP package is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read everything into memory to leverage archive/zip efficiently, 
	// assuming normal composer zips are relatively tiny.
	zipData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading upload", http.StatusInternalServerError)
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		http.Error(w, "Invalid ZIP archive", http.StatusBadRequest)
		return
	}

	var compJson map[string]interface{}
	found := false
	for _, zipFile := range zipReader.File {
		// Just find root or any matching composer.json
		if strings.HasSuffix(zipFile.Name, "composer.json") {
			rc, err := zipFile.Open()
			if err != nil {
				continue
			}
			jsonBytes, _ := io.ReadAll(rc)
			rc.Close()
			if json.Unmarshal(jsonBytes, &compJson) == nil {
				found = true
				break
			}
		}
	}

	if !found {
		http.Error(w, "composer.json not found inside zip", http.StatusBadRequest)
		return
	}

	nameIf, ok := compJson["name"]
	if !ok {
		http.Error(w, "composer.json is missing 'name' field", http.StatusBadRequest)
		return
	}
	pkgName := nameIf.(string)

	versionIf, ok := compJson["version"]
	if !ok {
		// fallback to CLI provided version
		manualVersion := r.FormValue("version")
		if manualVersion == "" {
			http.Error(w, "composer.json is missing 'version' and no form 'version' provided", http.StatusBadRequest)
			return
		}
		compJson["version"] = manualVersion
		versionIf = manualVersion
	}
	version := versionIf.(string)

	metaBytes, _ := json.Marshal(compJson)

	artifact := &db.UniversalArtifact{
		Format:      "composer",
		Namespace:   namespace,
		PackageName: pkgName,
		Version:     version,
		Metadata:    metaBytes,
	}

	artifactID, err := h.db.CreateArtifact(r.Context(), artifact)
	if err != nil {
		http.Error(w, "Failed to persist artifact", http.StatusInternalServerError)
		return
	}

	// Save dist using sha256 sum to guarantee uniqueness 
	hasher := sha256.New()
	hasher.Write(zipData)
	shaHex := hex.EncodeToString(hasher.Sum(nil))

	filename := fmt.Sprintf("%s-%s-%s.zip", filepath.Base(pkgName), version, shaHex[:8])
	path := filepath.Join("composer", namespace, pkgName, filename)

	writer, err := h.storage.Writer(r.Context(), path)
	if err != nil {
		http.Error(w, "Failed to allocate blob storage", http.StatusInternalServerError)
		return
	}
	_, _ = io.Copy(writer, bytes.NewReader(zipData))
	writer.Close()

	if err := h.db.AttachBlob(r.Context(), artifactID, shaHex, filename); err != nil {
		http.Error(w, "Failed attaching blob", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("PHP package %s version %s saved", pkgName, version)))
}
