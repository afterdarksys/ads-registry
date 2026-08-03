package v2

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

func digestFor(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func requestWithParams(method, target string, body io.Reader, params map[string]string) *http.Request {
	req := httptest.NewRequest(method, target, body)
	routeCtx := chi.NewRouteContext()
	for key, value := range params {
		routeCtx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func writeStorage(t *testing.T, provider storage.Provider, key string, content []byte) {
	t.Helper()
	w, err := provider.Writer(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func readStorage(t *testing.T, provider storage.Provider, key string) []byte {
	t.Helper()
	r, err := provider.Reader(context.Background(), key, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestHeadBlobRequiresRepositoryLocalObject(t *testing.T) {
	provider := storage.NewMemoryProvider()
	digest := digestFor([]byte("shared layer"))
	writeStorage(t, provider, storage.BlobPath("team/repo-a", digest), []byte("shared layer"))

	router := &Router{db: &db.MockStore{}, storage: provider}
	req := requestWithParams(http.MethodHead, "/v2/team/repo-b/blobs/"+digest, nil, map[string]string{"digest": digest})
	w := httptest.NewRecorder()

	router.headBlob(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("HEAD returned %d for a blob missing from the requested repository", w.Code)
	}
}

func TestGetBlobRangeHeadersAndBody(t *testing.T) {
	provider := storage.NewMemoryProvider()
	content := []byte("0123456789")
	digest := digestFor(content)
	writeStorage(t, provider, storage.BlobPath("team/repo", digest), content)

	router := &Router{db: &db.MockStore{}, storage: provider}
	req := requestWithParams(http.MethodGet, "/v2/team/repo/blobs/"+digest, nil, map[string]string{"digest": digest})
	req.Header.Set("Range", "bytes=3-6")
	w := httptest.NewRecorder()

	router.getBlob(w, req)
	if w.Code != http.StatusPartialContent {
		t.Fatalf("range GET returned %d", w.Code)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 3-6/10" {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "4" {
		t.Fatalf("Content-Length = %q", got)
	}
	if got := w.Body.String(); got != "3456" {
		t.Fatalf("body = %q", got)
	}
}

func TestPutManifestRejectsMissingBlob(t *testing.T) {
	provider := storage.NewMemoryProvider()
	missingDigest := digestFor([]byte("missing"))
	payload := []byte(`{"schemaVersion":2,"config":{"digest":"` + missingDigest + `"},"layers":[]}`)
	router := &Router{db: &db.MockStore{}, storage: provider}
	req := requestWithParams(http.MethodPut, "/v2/team/repo/manifests/latest", bytes.NewReader(payload), map[string]string{"reference": "latest"})
	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	w := httptest.NewRecorder()

	router.putManifest(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("manifest PUT returned %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("MANIFEST_BLOB_UNKNOWN")) {
		t.Fatalf("response did not identify missing blob: %s", w.Body.String())
	}
}

type firstMoveBlocker struct {
	storage.Provider
	blockedSource string
	entered       chan struct{}
	release       chan struct{}
	once          sync.Once
}

func (s *firstMoveBlocker) Move(ctx context.Context, source, target string) error {
	if source == s.blockedSource {
		s.once.Do(func() { close(s.entered) })
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.Provider.Move(ctx, source, target)
}

func TestSameDigestFinalizationsAreIsolatedByRepository(t *testing.T) {
	base := storage.NewMemoryProvider()
	content := []byte("identical layer")
	digest := digestFor(content)
	uuidA, uuidB := "upload-a", "upload-b"
	pathA := storage.UploadPath("team/repo-a", uuidA)
	pathB := storage.UploadPath("team/repo-b", uuidB)
	writeStorage(t, base, pathA, content)
	writeStorage(t, base, pathB, content)
	provider := &firstMoveBlocker{Provider: base, blockedSource: pathA, entered: make(chan struct{}), release: make(chan struct{})}
	router := &Router{db: &db.MockStore{}, storage: provider, uploadLocks: make(map[string]*uploadLock)}

	doneA := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := requestWithParams(http.MethodPut, "/v2/team/repo-a/blobs/uploads/"+uuidA+"?digest="+digest, nil, map[string]string{"uuid": uuidA})
		w := httptest.NewRecorder()
		router.putUpload(w, req)
		doneA <- w
	}()
	<-provider.entered

	doneB := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := requestWithParams(http.MethodPut, "/v2/team/repo-b/blobs/uploads/"+uuidB+"?digest="+digest, nil, map[string]string{"uuid": uuidB})
		w := httptest.NewRecorder()
		router.putUpload(w, req)
		doneB <- w
	}()

	select {
	case w := <-doneB:
		if w.Code != http.StatusCreated {
			t.Fatalf("repo B finalization returned %d: %s", w.Code, w.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("repo B finalization was incorrectly joined to repo A")
	}
	close(provider.release)
	if w := <-doneA; w.Code != http.StatusCreated {
		t.Fatalf("repo A finalization returned %d: %s", w.Code, w.Body.String())
	}

	if got := readStorage(t, base, storage.BlobPath("team/repo-a", digest)); !bytes.Equal(got, content) {
		t.Fatalf("repo A blob = %q", got)
	}
	if got := readStorage(t, base, storage.BlobPath("team/repo-b", digest)); !bytes.Equal(got, content) {
		t.Fatalf("repo B blob = %q", got)
	}
}

type gatedReader struct {
	content []byte
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	done    bool
}

func (r *gatedReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.once.Do(func() { close(r.entered) })
	<-r.release
	r.done = true
	return copy(p, r.content), nil
}

func TestPutWaitsForPatchBeforeDigestVerification(t *testing.T) {
	provider := storage.NewMemoryProvider()
	content := []byte("final layer bytes")
	digest := digestFor(content)
	uuid := "same-upload"
	writeStorage(t, provider, storage.UploadPath("team/repo", uuid), nil)
	router := &Router{db: &db.MockStore{}, storage: provider, uploadLocks: make(map[string]*uploadLock)}

	body := &gatedReader{content: content, entered: make(chan struct{}), release: make(chan struct{})}
	patchDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := requestWithParams(http.MethodPatch, "/v2/team/repo/blobs/uploads/"+uuid, body, map[string]string{"uuid": uuid})
		w := httptest.NewRecorder()
		router.patchUpload(w, req)
		patchDone <- w
	}()
	<-body.entered

	putDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := requestWithParams(http.MethodPut, "/v2/team/repo/blobs/uploads/"+uuid+"?digest="+digest, nil, map[string]string{"uuid": uuid})
		w := httptest.NewRecorder()
		router.putUpload(w, req)
		putDone <- w
	}()

	select {
	case w := <-putDone:
		t.Fatalf("PUT completed before PATCH released its upload lock: %d %s", w.Code, w.Body.String())
	case <-time.After(50 * time.Millisecond):
	}
	close(body.release)
	if w := <-patchDone; w.Code != http.StatusAccepted {
		t.Fatalf("PATCH returned %d: %s", w.Code, w.Body.String())
	}
	if w := <-putDone; w.Code != http.StatusCreated {
		t.Fatalf("PUT returned %d: %s", w.Code, w.Body.String())
	}
	if got := readStorage(t, provider, storage.BlobPath("team/repo", digest)); !bytes.Equal(got, content) {
		t.Fatalf("final blob content = %q", got)
	}
}

func TestPatchRejectsChunkThatWouldExceedTotalUploadLimit(t *testing.T) {
	provider := storage.NewMemoryProvider()
	uuid := "oversized-upload"
	writeStorage(t, provider, storage.UploadPath("team/repo", uuid), nil)
	router := &Router{db: &db.MockStore{}, storage: provider, uploadLocks: make(map[string]*uploadLock)}
	req := requestWithParams(http.MethodPatch, "/v2/team/repo/blobs/uploads/"+uuid, bytes.NewReader([]byte("x")), map[string]string{"uuid": uuid})
	req.ContentLength = maxUploadBytes + 1
	w := httptest.NewRecorder()

	router.patchUpload(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized PATCH returned %d: %s", w.Code, w.Body.String())
	}
}
