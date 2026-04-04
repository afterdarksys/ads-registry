package storage

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// MemoryProvider is an in-memory storage implementation for testing
type MemoryProvider struct {
	data  map[string][]byte
	mutex sync.RWMutex
}

func NewMemoryProvider() *MemoryProvider {
	return &MemoryProvider{
		data: make(map[string][]byte),
	}
}

func (m *MemoryProvider) Writer(ctx context.Context, path string) (io.WriteCloser, error) {
	return &memoryWriter{
		provider: m,
		path:     path,
		buffer:   &bytes.Buffer{},
	}, nil
}

func (m *MemoryProvider) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	data, exists := m.data[path]
	if !exists {
		return nil, io.EOF
	}

	if offset > int64(len(data)) {
		offset = int64(len(data))
	}

	return io.NopCloser(bytes.NewReader(data[offset:])), nil
}

func (m *MemoryProvider) Appender(ctx context.Context, path string) (io.WriteCloser, error) {
	return &memoryAppender{
		provider: m,
		path:     path,
		buffer:   &bytes.Buffer{},
	}, nil
}

func (m *MemoryProvider) Delete(ctx context.Context, path string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.data, path)
	return nil
}

func (m *MemoryProvider) Move(ctx context.Context, sourcePath, destPath string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	data, exists := m.data[sourcePath]
	if !exists {
		return io.EOF
	}

	m.data[destPath] = data
	delete(m.data, sourcePath)
	return nil
}

func (m *MemoryProvider) Stat(ctx context.Context, path string) (int64, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	data, exists := m.data[path]
	if !exists {
		return 0, io.EOF
	}

	return int64(len(data)), nil
}

type memoryWriter struct {
	provider *MemoryProvider
	path     string
	buffer   *bytes.Buffer
}

func (w *memoryWriter) Write(p []byte) (n int, err error) {
	return w.buffer.Write(p)
}

func (w *memoryWriter) Close() error {
	w.provider.mutex.Lock()
	defer w.provider.mutex.Unlock()
	w.provider.data[w.path] = w.buffer.Bytes()
	return nil
}

type memoryAppender struct {
	provider *MemoryProvider
	path     string
	buffer   *bytes.Buffer
}

func (a *memoryAppender) Write(p []byte) (n int, err error) {
	return a.buffer.Write(p)
}

func (a *memoryAppender) Close() error {
	a.provider.mutex.Lock()
	defer a.provider.mutex.Unlock()

	existing := a.provider.data[a.path]
	a.provider.data[a.path] = append(existing, a.buffer.Bytes()...)
	return nil
}
