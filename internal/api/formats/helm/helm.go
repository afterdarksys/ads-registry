package helm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	"gopkg.in/yaml.v3"
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

	r.Get("/index.yaml", h.getIndex)
	r.Post("/api/charts", h.uploadChart)
	r.Get("/charts/{filename}", h.downloadChart)

	return r
}

type ChartMetadata struct {
	ApiVersion  string   `yaml:"apiVersion,omitempty" json:"apiVersion,omitempty"`
	Name        string   `yaml:"name" json:"name"`
	Version     string   `yaml:"version" json:"version"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	AppVersion  string   `yaml:"appVersion,omitempty" json:"appVersion,omitempty"`
	Digest      string   `yaml:"digest,omitempty" json:"digest,omitempty"`
	Created     string   `yaml:"created,omitempty" json:"created,omitempty"`
	Urls        []string `yaml:"urls,omitempty" json:"urls,omitempty"`
}

type IndexContent struct {
	ApiVersion string                      `yaml:"apiVersion"`
	Entries    map[string][]ChartMetadata  `yaml:"entries"`
	Generated  string                      `yaml:"generated"`
}

func (h *Handler) getIndex(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	
	artifacts, err := h.db.SearchArtifacts(r.Context(), "helm", namespace, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	index := IndexContent{
		ApiVersion: "v1",
		Entries:    make(map[string][]ChartMetadata),
		Generated:  time.Now().Format(time.RFC3339),
	}

	protocol := "http"
	if r.TLS != nil {
		protocol = "https"
	}
	baseURL := fmt.Sprintf("%s://%s/repository/helm", protocol, r.Host)

	for _, artifact := range artifacts {
		if len(artifact.Metadata) == 0 {
			continue
		}

		var chartMeta ChartMetadata
		if err := json.Unmarshal(artifact.Metadata, &chartMeta); err != nil {
			continue
		}

		// Inject digest from the blob
		for _, blob := range artifact.Blobs {
			chartMeta.Digest = blob.BlobDigest
			chartMeta.Urls = []string{fmt.Sprintf("%s/charts/%s", baseURL, blob.FileName)}
		}
		
		chartMeta.Created = artifact.CreatedAt.Format(time.RFC3339)

		if _, ok := index.Entries[chartMeta.Name]; !ok {
			index.Entries[chartMeta.Name] = []ChartMetadata{}
		}
		index.Entries[chartMeta.Name] = append(index.Entries[chartMeta.Name], chartMeta)
	}

	w.Header().Set("Content-Type", "application/yaml")
	
	encoder := yaml.NewEncoder(w)
	defer encoder.Close()
	if err := encoder.Encode(index); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) uploadChart(w http.ResponseWriter, r *http.Request) {
	namespace := "default"

	// Read into a buffer because we need it twice: once for inspection, once for saving/digest
	// A Helm chart is typically small (kB or MBs). We enforce 50MB max.
	limitedReader := io.LimitReader(r.Body, 50*1024*1024)
	payload, err := io.ReadAll(limitedReader)
	if err != nil {
		http.Error(w, "Unable to read payload", http.StatusBadRequest)
		return
	}

	hasher := sha256.New()
	hasher.Write(payload)
	sha256Hex := hex.EncodeToString(hasher.Sum(nil))

	// Inspect the TGZ
	gzReader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		http.Error(w, "Invalid gzip format", http.StatusBadRequest)
		return
	}
	defer gzReader.Close()
	tarReader := tar.NewReader(gzReader)

	var chartMeta ChartMetadata
	foundChartYaml := false

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "Error parsing tarball", http.StatusBadRequest)
			return
		}

		// Look for Chart.yaml at the top level of the chart directory (e.g. "mychart/Chart.yaml")
		parts := strings.Split(header.Name, "/")
		if len(parts) == 2 && parts[1] == "Chart.yaml" {
			chartYamlData, err := io.ReadAll(tarReader)
			if err != nil {
				continue
			}
			err = yaml.Unmarshal(chartYamlData, &chartMeta)
			if err == nil {
				foundChartYaml = true
				break
			}
		}
	}

	if !foundChartYaml || chartMeta.Name == "" || chartMeta.Version == "" {
		http.Error(w, "Chart.yaml missing or invalid", http.StatusBadRequest)
		return
	}

	metaBytes, _ := json.Marshal(chartMeta)

	artifact := &db.UniversalArtifact{
		Format:      "helm",
		Namespace:   namespace,
		PackageName: chartMeta.Name,
		Version:     chartMeta.Version,
		Metadata:    metaBytes,
	}

	artifactID, err := h.db.CreateArtifact(r.Context(), artifact)
	if err != nil {
		http.Error(w, "Failed to register chart artifact", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("%s-%s.tgz", chartMeta.Name, chartMeta.Version)
	path := filepath.Join("helm", namespace, filename)

	writer, err := h.storage.Writer(r.Context(), path)
	if err != nil {
		http.Error(w, "Failed to initialize storage", http.StatusInternalServerError)
		return
	}
	_, err = io.Copy(writer, bytes.NewReader(payload))
	writer.Close()
	if err != nil {
		http.Error(w, "Failed to save chart", http.StatusInternalServerError)
		return
	}

	if err := h.db.AttachBlob(r.Context(), artifactID, sha256Hex, filename); err != nil {
		http.Error(w, "Failed to attach hash", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf(`{"saved": true}`)))
}

func (h *Handler) downloadChart(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	filename := chi.URLParam(r, "filename")

	path := filepath.Join("helm", namespace, filename)
	
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

	w.Header().Set("Content-Type", "application/x-tar")
	io.Copy(w, reader)
}
