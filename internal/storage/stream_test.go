package storage

import (
	"bytes"
	"io"
	"testing"
)

type trackedReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackedReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestStreamWriterStreamsPrefixAndNewContent(t *testing.T) {
	prefix := &trackedReadCloser{Reader: bytes.NewBufferString("existing-")}
	var uploaded []byte
	w := NewStreamWriter(func(body io.Reader) error {
		var err error
		uploaded, err = io.ReadAll(body)
		return err
	}, prefix)

	if _, err := io.WriteString(w, "new"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got := string(uploaded); got != "existing-new" {
		t.Fatalf("uploaded %q", got)
	}
	if !prefix.closed {
		t.Fatal("prefix reader was not closed")
	}
}

func TestRepositoryPathDoesNotDuplicateNamespace(t *testing.T) {
	if got := RepositoryPath("team", "team/repo"); got != "team/repo" {
		t.Fatalf("qualified repository became %q", got)
	}
	if got := RepositoryPath("team", "repo"); got != "team/repo" {
		t.Fatalf("unqualified repository became %q", got)
	}
}
