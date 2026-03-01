package local

import (
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
	// O_TRUNC for simplicity. In a real registry we'd use temporary files and rename.
	return os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
}

func (s *LocalStore) Appender(ctx context.Context, path string) (io.WriteCloser, error) {
	fullPath := s.resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
