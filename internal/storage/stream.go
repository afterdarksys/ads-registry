package storage

import (
	"io"
	"sync"
)

// NewStreamWriter adapts a blocking streaming upload function to io.WriteCloser.
// prefix, when non-nil, is streamed before newly written bytes and is always closed.
func NewStreamWriter(upload func(io.Reader) error, prefix io.ReadCloser) io.WriteCloser {
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	body := io.Reader(reader)
	if prefix != nil {
		body = io.MultiReader(prefix, reader)
	}
	go func() {
		err := upload(body)
		if prefix != nil {
			if closeErr := prefix.Close(); err == nil {
				err = closeErr
			}
		}
		reader.CloseWithError(err)
		done <- err
	}()
	return &streamWriter{writer: writer, done: done}
}

type streamWriter struct {
	writer    *io.PipeWriter
	done      <-chan error
	closeOnce sync.Once
	closeErr  error
}

func (w *streamWriter) Write(content []byte) (int, error) {
	return w.writer.Write(content)
}

func (w *streamWriter) Close() error {
	w.closeOnce.Do(func() {
		pipeErr := w.writer.Close()
		uploadErr := <-w.done
		if uploadErr != nil {
			w.closeErr = uploadErr
		} else {
			w.closeErr = pipeErr
		}
	})
	return w.closeErr
}
