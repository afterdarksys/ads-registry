package sync

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestSyncBlobReadsRepositoryLocalPath(t *testing.T) {
	content := []byte("peer layer")
	digest := "sha256:1c16ef2f65604ef8f4d7f9490ac5f999c3dd8de79328ace0db268dfb28234216"
	provider := storage.NewMemoryProvider()
	w, err := provider.Writer(context.Background(), storage.BlobPath("team/repo", digest))
	if err != nil {
		t.Fatal(err)
	}
	w.Write(content)
	w.Close()

	var uploaded []byte
	manager := NewManager(nil, nil, &db.MockStore{}, provider, "")
	manager.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := &http.Response{Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request}
		switch request.Method {
		case http.MethodHead:
			response.StatusCode = http.StatusNotFound
		case http.MethodPost:
			response.Header.Set("Location", "/v2/team/repo/blobs/uploads/session")
			response.StatusCode = http.StatusAccepted
		case http.MethodPut:
			uploaded, _ = io.ReadAll(request.Body)
			response.StatusCode = http.StatusCreated
		default:
			response.StatusCode = http.StatusMethodNotAllowed
		}
		return response, nil
	})}

	peer := config.PeerRegistry{Name: "test", Endpoint: "https://peer.test", Token: "token"}
	if err := manager.syncBlob(context.Background(), digest, "team/repo", peer); err != nil {
		t.Fatal(err)
	}
	if string(uploaded) != string(content) {
		t.Fatalf("uploaded %q", uploaded)
	}
}

func TestSyncBlobRejectsMalformedDigestWithoutPanic(t *testing.T) {
	manager := NewManager(nil, nil, &db.MockStore{}, storage.NewMemoryProvider(), "")
	err := manager.syncBlob(context.Background(), "sha256:x", "team/repo", config.PeerRegistry{})
	if err == nil || !strings.Contains(err.Error(), "invalid blob digest") {
		t.Fatalf("unexpected error: %v", err)
	}
}
