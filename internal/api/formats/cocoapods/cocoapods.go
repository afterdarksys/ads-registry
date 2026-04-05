package cocoapods

import (
	"crypto/sha256"
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

	r.Post("/api/v1/pods", h.uploadPod)
	r.Get("/all_pods.txt", h.allPodsIndex)
	
	// Complex wildcard catch
	r.Get("/Specs/*", h.podSpecView)
	
	// Endpoint for direct downloaded binaries if "source.http" points back to Registry
	r.Get("/tarballs/{filename}", h.downloadTarball)

	return r
}

func (h *Handler) allPodsIndex(w http.ResponseWriter, r *http.Request) {
	namespace := "default"

	// Fetch all pods from Postgres
	artifacts, err := h.db.SearchArtifacts(r.Context(), "cocoapods", namespace, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	uniquePods := make(map[string]bool)
	for _, a := range artifacts {
		uniquePods[a.PackageName] = true
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for podName := range uniquePods {
		fmt.Fprintf(w, "%s\n", podName)
	}
}

func (h *Handler) podSpecView(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	
	// The path arrives as something like 1/2/3/Name/1.0.0/Name.podspec.json
	path := chi.URLParam(r, "*")
	parts := strings.Split(path, "/")

	if len(parts) < 3 {
		http.Error(w, "Invalid Spec path", http.StatusBadRequest)
		return
	}

	// name is typically the second to last folder, or filename minus .podspec.json
	fileName := parts[len(parts)-1]
	if !strings.HasSuffix(fileName, ".podspec.json") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	
	// Remove .podspec.json to get podName
	podName := strings.TrimSuffix(fileName, ".podspec.json")
	
	// Version is the folder name directly above the file
	version := parts[len(parts)-2]

	artifact, err := h.db.GetArtifact(r.Context(), "cocoapods", namespace, podName, version)
	if err != nil || len(artifact.Metadata) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(artifact.Metadata)
}

func (h *Handler) uploadPod(w http.ResponseWriter, r *http.Request) {
	namespace := "default"

	err := r.ParseMultipartForm(500 << 20)
	if err != nil {
		http.Error(w, "Invalid upload format", http.StatusBadRequest)
		return
	}

	// Grab spec
	specJson := r.FormValue("podspec")
	if specJson == "" {
		http.Error(w, "podspec text parameter is required", http.StatusBadRequest)
		return
	}

	var parsedSpec map[string]interface{}
	if err := json.Unmarshal([]byte(specJson), &parsedSpec); err != nil {
		http.Error(w, "Invalid JSON podspec", http.StatusBadRequest)
		return
	}

	podName, _ := parsedSpec["name"].(string)
	version, _ := parsedSpec["version"].(string)

	if podName == "" || version == "" {
		http.Error(w, "podspec missing name or version", http.StatusBadRequest)
		return
	}

	// Determine if user uploaded binary tarball attached
	tarballFile, header, err := r.FormFile("tarball")
	
	// If the tarball exists, rewrite the parsedSpec "source: { http: 'URL' }" to dynamically
	// point directly to our /tarballs/ caching layer!
	var sha2Hex string
	var filename string
	if err == nil {
		defer tarballFile.Close()
		filename = fmt.Sprintf("%s-%s-%s", podName, version, header.Filename)
		
		hasher := sha256.New()
		storagePath := filepath.Join("cocoapods", namespace, podName, filename)
		writer, wErr := h.storage.Writer(r.Context(), storagePath)
		if wErr == nil {
			tee := io.TeeReader(tarballFile, hasher)
			io.Copy(writer, tee)
			writer.Close()
			sha2Hex = hex.EncodeToString(hasher.Sum(nil))

			protocol := "http"
			if r.TLS != nil {
				protocol = "https"
			}
			baseURL := fmt.Sprintf("%s://%s/repository/cocoapods", protocol, r.Host)

			parsedSpec["source"] = map[string]string{
				"http": fmt.Sprintf("%s/tarballs/%s", baseURL, filename),
			}
			
			// serialize it out
			newBytes, _ := json.Marshal(parsedSpec)
			specJson = string(newBytes)
		}
	}

	// Persist
	artifact := &db.UniversalArtifact{
		Format:      "cocoapods",
		Namespace:   namespace,
		PackageName: podName,
		Version:     version,
		Metadata:    []byte(specJson),
	}

	artifactID, err := h.db.CreateArtifact(r.Context(), artifact)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	if sha2Hex != "" && filename != "" {
		h.db.AttachBlob(r.Context(), artifactID, sha2Hex, filename)
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf(`{"success": true, "pod": "%s", "version": "%s"}`, podName, version)))
}

func (h *Handler) downloadTarball(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	namespace := "default"

	// Reconstruct path. The format we used above maps as "cocoapods/namespace/podName/filename"
	// However, we don't have podName directly in the path template parameter.
	// But our filename was formatted as: "podName-version-original.tgz".
	// Let's split on `-` to deduce podName.
	parts := strings.Split(filename, "-")
	if len(parts) < 3 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	podName := parts[0]

	path := filepath.Join("cocoapods", namespace, podName, filename)

	reader, err := h.storage.Reader(r.Context(), path, 0)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/x-gzip")
	io.Copy(w, reader)
}
