package runtime

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PackBuildContext creates a tar archive of the specified path and writes it to the writer.
func PackBuildContext(src string, w io.Writer) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	if !strings.HasSuffix(src, string(filepath.Separator)) {
		src = src + string(filepath.Separator)
	}

	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// Ensure the path relative to the root of the context
		relPath := strings.TrimPrefix(file, src)
		if relPath == "" {
			return nil // Root dir
		}

		header.Name = filepath.ToSlash(relPath)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !fi.IsDir() {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			// Must close immediately, not defer in loop
			_, copyErr := io.Copy(tw, f)
			f.Close()
			if copyErr != nil {
				return copyErr
			}
		}
		return nil
	})
}
