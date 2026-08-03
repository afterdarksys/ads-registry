package scanner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	godigest "github.com/opencontainers/go-digest"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

type layerDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
}

type imageManifest struct {
	Layers []layerDescriptor `json:"layers"`
}

// ExtractImageLayers resolves an image manifest and safely expands each of its
// repository-local layers into destDir for embedded scanners.
func ExtractImageLayers(ctx context.Context, database db.Store, provider storage.Provider, namespace, repository, manifestDigest, destDir string) error {
	fullRepo := storage.RepositoryPath(namespace, repository)
	_, _, payload, err := database.GetManifest(ctx, fullRepo, manifestDigest)
	if err != nil {
		return fmt.Errorf("get manifest %s: %w", manifestDigest, err)
	}

	var manifest imageManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return fmt.Errorf("parse image manifest: %w", err)
	}
	if len(manifest.Layers) == 0 {
		return fmt.Errorf("manifest %s has no image layers", manifestDigest)
	}

	for _, layer := range manifest.Layers {
		parsed, err := godigest.Parse(layer.Digest)
		if err != nil {
			return fmt.Errorf("invalid layer digest %q: %w", layer.Digest, err)
		}
		reader, err := provider.Reader(ctx, storage.BlobPath(fullRepo, parsed.String()), 0)
		if err != nil {
			return fmt.Errorf("open layer %s: %w", parsed, err)
		}
		err = extractLayerArchive(reader, layer.MediaType, destDir)
		closeErr := reader.Close()
		if err != nil {
			return fmt.Errorf("extract layer %s: %w", parsed, err)
		}
		if closeErr != nil {
			return fmt.Errorf("close layer %s: %w", parsed, closeErr)
		}
	}
	return nil
}

func extractLayerArchive(source io.Reader, mediaType, destDir string) error {
	var archiveReader io.Reader = source
	var closeDecompressor func() error
	switch {
	case strings.Contains(mediaType, "+gzip"):
		gz, err := gzip.NewReader(source)
		if err != nil {
			return err
		}
		archiveReader = gz
		closeDecompressor = gz.Close
	case strings.Contains(mediaType, "+zstd"):
		decoder, err := zstd.NewReader(source)
		if err != nil {
			return err
		}
		archiveReader = decoder
		closeDecompressor = func() error { decoder.Close(); return nil }
	}
	if closeDecompressor != nil {
		defer closeDecompressor()
	}

	tarReader := tar.NewReader(archiveReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		target, err := safeExtractionPath(destDir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode).Perm()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode).Perm())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, tarReader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink, tar.TypeLink:
			// Links are unnecessary for content scanning and can escape the
			// extraction root when later tools traverse the directory.
			continue
		}
	}
}

func safeExtractionPath(root, name string) (string, error) {
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe layer path %q", name)
	}
	target := filepath.Join(root, cleanName)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("layer path escapes extraction root: %q", name)
	}
	return target, nil
}
