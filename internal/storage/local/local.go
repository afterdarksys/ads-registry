package local

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/ryan/ads-registry/internal/storage"
)

type LocalStore struct {
	rootDir string
}

func New(rootDir string) (*LocalStore, error) {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, err
	}
	return &LocalStore{rootDir: rootDir}, nil
}

func (s *LocalStore) resolvePath(path string) string {
	return filepath.Join(s.rootDir, path)
}

func (s *LocalStore) Writer(ctx context.Context, path string) (io.WriteCloser, error) {
	fullPath := s.resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}

	// Use buffered I/O for Writer as well
	return &bufferedFileWriter{
		file:   file,
		writer: bufio.NewWriterSize(file, 256*1024),
	}, nil
}

// bufferedFileWriter wraps os.File with buffered I/O and explicit sync
type bufferedFileWriter struct {
	file   *os.File
	writer *bufio.Writer
}

func (w *bufferedFileWriter) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

func (w *bufferedFileWriter) Close() error {
	// Flush buffered data to OS
	if err := w.writer.Flush(); err != nil {
		w.file.Close()
		return err
	}
	// Ensure data is written to disk before closing
	if err := w.file.Sync(); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

func (s *LocalStore) Appender(ctx context.Context, path string) (io.WriteCloser, error) {
	fullPath := s.resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	// Use 256KB buffer for optimal network-to-disk performance
	// Larger than TCP buffer (1MB) would waste memory, smaller causes more syscalls
	return &bufferedFileWriter{
		file:   file,
		writer: bufio.NewWriterSize(file, 256*1024),
	}, nil
}

func (s *LocalStore) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	fullPath := s.resolvePath(path)
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func (s *LocalStore) Delete(ctx context.Context, path string) error {
	fullPath := s.resolvePath(path)
	err := os.Remove(fullPath)
	if os.IsNotExist(err) {
		return storage.ErrNotFound
	}
	return err
}

func (s *LocalStore) Move(ctx context.Context, source string, target string) error {
	fullSource := s.resolvePath(source)
	fullTarget := s.resolvePath(target)

	if err := os.MkdirAll(filepath.Dir(fullTarget), 0755); err != nil {
		return err
	}

	return os.Rename(fullSource, fullTarget)
}

func (s *LocalStore) Stat(ctx context.Context, path string) (int64, error) {
	fullPath := s.resolvePath(path)
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, storage.ErrNotFound
		}
		return 0, err
	}
	return info.Size(), nil
}
