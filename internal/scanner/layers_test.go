package scanner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

type manifestStore struct {
	db.MockStore
	payload []byte
}

func (s *manifestStore) GetManifest(context.Context, string, string) (string, string, []byte, error) {
	return "application/vnd.oci.image.manifest.v1+json", "", s.payload, nil
}

func layerDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func gzipLayer(t *testing.T, name, content string) []byte {
	t.Helper()
	var result bytes.Buffer
	gz := gzip.NewWriter(&result)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func TestExtractImageLayersUsesRepositoryLocalLayer(t *testing.T) {
	layer := gzipLayer(t, "app/config.txt", "safe")
	digest := layerDigest(layer)
	database := &manifestStore{payload: []byte(fmt.Sprintf(`{"schemaVersion":2,"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":%q}]}`, digest))}
	provider := storage.NewMemoryProvider()
	w, err := provider.Writer(context.Background(), storage.BlobPath("team/repo", digest))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(layer); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := ExtractImageLayers(context.Background(), database, provider, "team", "team/repo", layerDigest([]byte("manifest")), dest); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "app", "config.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "safe" {
		t.Fatalf("extracted content = %q", content)
	}
}

func TestLayerExtractionRejectsTraversal(t *testing.T) {
	layer := gzipLayer(t, "../../escape", "bad")
	digest := layerDigest(layer)
	database := &manifestStore{payload: []byte(fmt.Sprintf(`{"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":%q}]}`, digest))}
	provider := storage.NewMemoryProvider()
	w, _ := provider.Writer(context.Background(), storage.BlobPath("team/repo", digest))
	w.Write(layer)
	w.Close()

	err := ExtractImageLayers(context.Background(), database, provider, "team", "repo", "sha256:manifest", t.TempDir())
	if err == nil {
		t.Fatal("expected traversal layer to be rejected")
	}
}
