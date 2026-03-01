package storage

import (
	"context"
	"errors"
	"io"
)

var (
	ErrNotFound = errors.New("blob not found")
)

type Provider interface {
	Writer(ctx context.Context, path string) (io.WriteCloser, error)
	Appender(ctx context.Context, path string) (io.WriteCloser, error)
	Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Move(ctx context.Context, source string, target string) error
	Stat(ctx context.Context, path string) (int64, error)
}
