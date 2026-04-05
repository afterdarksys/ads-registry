package apt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
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

	// Upload API
	r.Post("/upload", h.uploadPackage)

	// APT Standard Directory Layout API
	r.Get("/dists/{codename}/Release", h.getRelease)
	r.Get("/dists/{codename}/main/binary-{arch}/Packages", h.getPackages)
	r.Get("/dists/{codename}/main/binary-{arch}/Packages.gz", h.getPackagesGz)
	
	// Binary Download
	r.Get("/pool/main/{prefix}/{pkgName}/{filename}", h.downloadPackage)

	return r
}

func (h *Handler) uploadPackage(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	
	// Assume the user sets ?codename=foo&arch=amd64 optionally, default to xenial + amd64
	codename := r.URL.Query().Get("codename")
	if codename == "" {
		codename = "stable"
	}

	payload, err := io.ReadAll(io.LimitReader(r.Body, 500*1024*1024)) // up to 500MB deb
	if err != nil {
		http.Error(w, "Error reading upload", http.StatusBadRequest)
		return
	}

	// Native Minimal 'ar' extraction to find control.tar.gz
	controlData, err := extractControlDotTarDotGz(payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid debian package / parsing error: %v", err), http.StatusBadRequest)
		return
	}

	// Parse 'control' file from control.tar.gz
	dcfProperties, err := parseControlFromTarGz(controlData)
	if err != nil || dcfProperties["Package"] == "" {
		http.Error(w, "Missing or invalid control file inside package", http.StatusBadRequest)
		return
	}

	pkgName := dcfProperties["Package"]
	version := dcfProperties["Version"]
	arch := dcfProperties["Architecture"]
	if arch == "" {
		arch = "amd64"
	}

	// Calculate Hashes dynamically for the APT manifests
	dcfProperties["Size"] = strconv.Itoa(len(payload))
	md5sum := md5.Sum(payload)
	dcfProperties["MD5sum"] = hex.EncodeToString(md5sum[:])
	sha256sum := sha256.Sum256(payload)
	dcfProperties["SHA256"] = hex.EncodeToString(sha256sum[:])

	// Force correct Filename property mapping in APT Packages DB
	prefix := string(pkgName[0])
	filename := fmt.Sprintf("%s_%s_%s.deb", pkgName, version, arch)
	poolPath := fmt.Sprintf("pool/main/%s/%s/%s", prefix, pkgName, filename)
	dcfProperties["Filename"] = poolPath
	dcfProperties["Codename"] = codename // track which codename it belongs to

	metaBytes, _ := json.Marshal(dcfProperties)

	artifact := &db.UniversalArtifact{
		Format:      "apt",
		Namespace:   namespace,
		PackageName: pkgName,
		Version:     version,
		Metadata:    metaBytes,
	}

	artifactID, err := h.db.CreateArtifact(r.Context(), artifact)
	if err != nil {
		http.Error(w, "Failed to record artifact index", http.StatusInternalServerError)
		return
	}

	writer, err := h.storage.Writer(r.Context(), filepath.Join("apt", namespace, poolPath))
	if err != nil {
		http.Error(w, "Storage failed", http.StatusInternalServerError)
		return
	}
	_, _ = io.Copy(writer, bytes.NewReader(payload))
	writer.Close()

	if err := h.db.AttachBlob(r.Context(), artifactID, dcfProperties["SHA256"], filename); err != nil {
		http.Error(w, "Failed attaching blob data", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("Uploaded successfully as %s", poolPath)))
}

func (h *Handler) getPackages(w http.ResponseWriter, r *http.Request) {
	manifest, _ := h.buildPackagesManifest(r.Context(), r)
	w.Header().Set("Content-Type", "text/plain")
	w.Write(manifest)
}

func (h *Handler) getPackagesGz(w http.ResponseWriter, r *http.Request) {
	manifest, _ := h.buildPackagesManifest(r.Context(), r)
	w.Header().Set("Content-Type", "application/x-gzip")
	gw := gzip.NewWriter(w)
	gw.Write(manifest)
	gw.Close()
}

func (h *Handler) buildPackagesManifest(ctx context.Context, r *http.Request) ([]byte, error) {
	namespace := "default"
	targetCodename := chi.URLParam(r, "codename")
	targetArch := chi.URLParam(r, "arch")

	artifacts, err := h.db.SearchArtifacts(ctx, "apt", namespace, nil)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	for _, artifact := range artifacts {
		var meta map[string]string
		if err := json.Unmarshal(artifact.Metadata, &meta); err == nil {
			if meta["Codename"] != targetCodename {
				continue
			}
			if meta["Architecture"] != "all" && meta["Architecture"] != targetArch {
				continue
			}
			
			// Render the Debian Control format exactly from map
			// Ensure essential fields run first
			for _, key := range []string{"Package", "Version", "Architecture", "Maintainer", "Depends", "Description", "Filename", "Size", "MD5sum", "SHA256"} {
				if val, ok := meta[key]; ok {
					buf.WriteString(fmt.Sprintf("%s: %s\n", key, val))
				}
			}
			buf.WriteString("\n")
		}
	}
	return buf.Bytes(), nil
}

func (h *Handler) getRelease(w http.ResponseWriter, r *http.Request) {
	codename := chi.URLParam(r, "codename")
	
	// Create dummy Packages text strictly for hash generation. Must match exactly.
	// Since we mock multiple architectures for now, let's just assert amd64
	
	// In reality we should fetch the raw generated package bytes via a cache mapping, 
	// but to prevent performance delays, we will directly construct dynamic metadata.
	
	// Workaround: Mock generation by making a dummy inner query
	// R needs to be a chi request map
	rPath := chi.NewRouteContext()
	rPath.URLParams.Add("codename", codename)
	rPath.URLParams.Add("arch", "amd64")
	req := r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rPath))
	
	packagesRaw, _ := h.buildPackagesManifest(r.Context(), req)

	// gzip them
	var gzBuf bytes.Buffer
	gzW := gzip.NewWriter(&gzBuf)
	gzW.Write(packagesRaw)
	gzW.Close()
	packagesGzRaw := gzBuf.Bytes()

	// Calc hashes
	rawSize := len(packagesRaw)
	rawMD5 := hex.EncodeToString(md5sum(packagesRaw))
	rawSHA2 := hex.EncodeToString(sha256sum(packagesRaw))

	gzSize := len(packagesGzRaw)
	gzMD5 := hex.EncodeToString(md5sum(packagesGzRaw))
	gzSHA2 := hex.EncodeToString(sha256sum(packagesGzRaw))

	dcf := []string{
		fmt.Sprintf("Origin: ADS Registry"),
		fmt.Sprintf("Label: ADS"),
		fmt.Sprintf("Codename: %s", codename),
		fmt.Sprintf("Architectures: amd64"),
		fmt.Sprintf("Components: main"),
		fmt.Sprintf("Description: Ads Universal Artifact Registry"),
		fmt.Sprintf("MD5Sum:"),
		fmt.Sprintf(" %s %30d main/binary-amd64/Packages", rawMD5, rawSize),
		fmt.Sprintf(" %s %30d main/binary-amd64/Packages.gz", gzMD5, gzSize),
		fmt.Sprintf("SHA256:"),
		fmt.Sprintf(" %s %30d main/binary-amd64/Packages", rawSHA2, rawSize),
		fmt.Sprintf(" %s %30d main/binary-amd64/Packages.gz", gzSHA2, gzSize),
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(strings.Join(dcf, "\n") + "\n"))
}

func (h *Handler) downloadPackage(w http.ResponseWriter, r *http.Request) {
	namespace := "default"
	prefix := chi.URLParam(r, "prefix")
	pkgName := chi.URLParam(r, "pkgName")
	filename := chi.URLParam(r, "filename")

	poolPath := fmt.Sprintf("pool/main/%s/%s/%s", prefix, pkgName, filename)
	path := filepath.Join("apt", namespace, poolPath)

	reader, err := h.storage.Reader(r.Context(), path, 0)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/vnd.debian.binary-package")
	io.Copy(w, reader)
}

// ---- Helpers ----

func md5sum(data []byte) []byte {
	m := md5.Sum(data)
	return m[:]
}
func sha256sum(data []byte) []byte {
	s := sha256.Sum256(data)
	return s[:]
}

// extractControlDotTarDotGz scans a lightweight Unix `ar` file array memory map seeking the `control.tar.gz` object
func extractControlDotTarDotGz(payload []byte) ([]byte, error) {
	if !bytes.HasPrefix(payload, []byte("!<arch>\n")) {
		return nil, fmt.Errorf("not an ar archive")
	}

	offset := 8
	for offset < len(payload) {
		if offset+60 > len(payload) {
			break
		}
		
		name := strings.TrimSpace(string(payload[offset : offset+16]))
		sizeStr := strings.TrimSpace(string(payload[offset+48 : offset+58]))
		size, err := strconv.Atoi(sizeStr)
		if err != nil {
			return nil, err
		}

		offset += 60 // skip header

		if name == "control.tar.gz" {
			if offset+size > len(payload) {
				return nil, fmt.Errorf("corrupted ar archive")
			}
			return payload[offset : offset+size], nil
		}

		offset += size
		if size%2 != 0 {
			offset++
		}
	}
	return nil, fmt.Errorf("control.tar.gz not found")
}

// parseControlFromTarGz unzips the control.tar.gz and reads the nested 'control' manifest text
func parseControlFromTarGz(data []byte) (map[string]string, error) {
	gzR, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gzR.Close()
	tarR := tar.NewReader(gzR)

	for {
		hdr, err := tarR.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		// Typically just named `./control`
		if strings.HasSuffix(hdr.Name, "control") {
			content, err := io.ReadAll(tarR)
			if err != nil {
				return nil, err
			}
			
			// Parse key: value
			props := make(map[string]string)
			lines := strings.Split(string(content), "\n")
			var lastKey string
			for _, line := range lines {
				if len(line) == 0 {
					continue
				}
				if line[0] == ' ' || line[0] == '\t' {
					// multiline continuation (like Description)
					if lastKey != "" {
						props[lastKey] = props[lastKey] + "\n" + line
					}
					continue
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					props[key] = val
					lastKey = key
				}
			}
			return props, nil
		}
	}
	return nil, fmt.Errorf("control file not found inside tarball")
}
