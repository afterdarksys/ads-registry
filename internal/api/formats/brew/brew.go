package brew

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

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

	r.Post("/upload", h.uploadBottle)
	r.Get("/api/formula/{formula}.json", h.bottleMetadata)
	r.Get("/{filename}", h.downloadBottle)

	return r
}

func (h *Handler) bottleMetadata(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	formulaName := chi.URLParam(r, "formula")

	// Get latest version or all versions for this formula
	artifacts, err := h.db.ListArtifacts(r.Context(), "brew", namespace, formulaName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(artifacts) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Homebrew API returns specific latest version properties 
	// We'll emulate a modern Formula response
	var formula map[string]interface{}
	
	// Assume the first one is the newest/latest
	artifact := artifacts[0]
	if len(artifact.Metadata) > 0 {
		json.Unmarshal(artifact.Metadata, &formula)
	} else {
		formula = make(map[string]interface{})
	}

	// Ensure the base structural pointers are correctly aligned with our registry
	formula["name"] = formulaName
	formula["versions"] = map[string]interface{}{
		"stable": artifact.Version,
		"head":   artifact.Version,
	}

	// Map the bottle URLs
	if _, ok := formula["bottle"]; !ok {
		formula["bottle"] = map[string]interface{}{"stable": map[string]interface{}{"files": map[string]interface{}{}}}
	}
	
	protocol := "http"
	if r.TLS != nil {
		protocol = "https"
	}
	baseURL := fmt.Sprintf("%s://%s/repository/brew", protocol, r.Host)

	// Inject the dynamic blobs back into the bottle definition
	stableMap := formula["bottle"].(map[string]interface{})["stable"].(map[string]interface{})
	if _, ok := stableMap["files"]; !ok {
		stableMap["files"] = make(map[string]interface{})
	}
	fileMap := stableMap["files"].(map[string]interface{})

	for _, blob := range artifact.Blobs {
		// Use standard mappings or derive architecture tag from DB
		// Let's assume generic "all" or specific tag saved in the filename
		fileMap["all"] = map[string]interface{}{
			"url":    fmt.Sprintf("%s/%s", baseURL, blob.FileName),
			"sha256": blob.BlobDigest,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(formula)
}

func (h *Handler) uploadBottle(w http.ResponseWriter, r *http.Request) {
	namespace := "default"

	err := r.ParseMultipartForm(500 << 20) // 500MB
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	formulaName := r.FormValue("name")
	version := r.FormValue("version")
	formulaJson := r.FormValue("json")

	if formulaName == "" || version == "" {
		http.Error(w, "Name and version required", http.StatusBadRequest)
		return
	}

	bottleFile, header, err := r.FormFile("bottle")
	if err != nil {
		http.Error(w, "Bottle file required", http.StatusBadRequest)
		return
	}
	defer bottleFile.Close()

	if formulaJson == "" {
		formulaJson = "{}"
	}

	artifact := &db.UniversalArtifact{
		Format:      "brew",
		Namespace:   namespace,
		PackageName: formulaName,
		Version:     version,
		Metadata:    []byte(formulaJson),
	}

	artifactID, err := h.db.CreateArtifact(r.Context(), artifact)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	hasher := sha256.New()
	path := filepath.Join("brew", namespace, formulaName, version, header.Filename)
	writer, err := h.storage.Writer(r.Context(), path)
	if err != nil {
		http.Error(w, "Storage failed", http.StatusInternalServerError)
		return
	}

	tee := io.TeeReader(bottleFile, hasher)
	if _, err := io.Copy(writer, tee); err != nil {
		writer.Close()
		http.Error(w, "File upload failed", http.StatusInternalServerError)
		return
	}
	writer.Close()

	sha2Hex := hex.EncodeToString(hasher.Sum(nil))

	if err := h.db.AttachBlob(r.Context(), artifactID, sha2Hex, header.Filename); err != nil {
		http.Error(w, "Failed to attach hash", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Bottle ingested successfully"))
}

func (h *Handler) downloadBottle(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	namespace := "default"

	// Find the blob via database to get the exact storage path since we nest them under formulaName/version
	// Actually, an easier way is to map the filename logically, but let's just search the DB if necessary.
	// For MVP, we pass formulaName and Version in the Filename or construct the search. 
	// To support root-level /name-1.0.tar.gz downloads without full path, we query artifact_blobs by filename!
	
	// Because storage needs exact paths, and we saved it as `brew/namespace/formula/version/filename`,
	// we will lookup the artifact by searching all brew artifacts to find the matching blob.
	// We'll emulate a simple path resolution query:
	
	artifacts, err := h.db.SearchArtifacts(r.Context(), "brew", namespace, nil)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var foundPath string
	for _, a := range artifacts {
		for _, b := range a.Blobs {
			if b.FileName == filename {
				foundPath = filepath.Join("brew", namespace, a.PackageName, a.Version, filename)
				break
			}
		}
	}

	if foundPath == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	reader, err := h.storage.Reader(r.Context(), foundPath, 0)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/gzip")
	io.Copy(w, reader)
}
