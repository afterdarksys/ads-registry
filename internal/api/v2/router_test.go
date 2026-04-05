package v2

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

// TestRouterPathMatching verifies that chi router correctly matches
// both single-level and multi-level repository paths for all endpoints
func TestRouterPathMatching(t *testing.T) {
	// Create a minimal router setup
	dbStore := &db.MockStore{}
	storageProvider := storage.NewMemoryProvider()

	// Create a test token service with minimal config
	authCfg := config.AuthConfig{
		TokenIssuer:  "test-registry",
		TokenService: "registry",
	}
	tokenService, err := auth.NewTokenService(authCfg)
	if err != nil {
		t.Fatalf("Failed to create token service: %v", err)
	}

	router := NewRouter(dbStore, storageProvider, tokenService, nil, nil, nil, nil, nil, false, nil)

	mux := chi.NewRouter()
	router.Register(mux)

	testCases := []struct {
		name           string
		method         string
		path           string
		expectStatus   int
		expectHandler  string
	}{
		// Single-level repository paths
		{
			name:          "Single-level: POST /v2/tokenworx/blobs/uploads/",
			method:        "POST",
			path:          "/v2/tokenworx/blobs/uploads/",
			expectStatus:  http.StatusUnauthorized, // Auth middleware will reject (no token)
			expectHandler: "startUpload",
		},
		{
			name:          "Single-level: GET /v2/alpine/tags/list",
			method:        "GET",
			path:          "/v2/alpine/tags/list",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "getTags",
		},
		{
			name:          "Single-level: GET /v2/nginx/manifests/latest",
			method:        "GET",
			path:          "/v2/nginx/manifests/latest",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "getManifest",
		},
		{
			name:          "Single-level missing trailing slash: POST /v2/tokenworx/blobs/uploads",
			method:        "POST",
			path:          "/v2/tokenworx/blobs/uploads",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "startUpload",
		},
		{
			name:          "Single-level delete manifest: DELETE /v2/nginx/manifests/latest",
			method:        "DELETE",
			path:          "/v2/nginx/manifests/latest",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "deleteManifest",
		},

		// Two-level repository paths
		{
			name:          "Two-level: POST /v2/web3dns/aiserve-farm/blobs/uploads/",
			method:        "POST",
			path:          "/v2/web3dns/aiserve-farm/blobs/uploads/",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "startUpload",
		},
		{
			name:          "Two-level: GET /v2/acme/myapp/tags/list",
			method:        "GET",
			path:          "/v2/acme/myapp/tags/list",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "getTags",
		},

		// Three-level repository paths
		{
			name:          "Three-level: POST /v2/org/team/app/blobs/uploads/",
			method:        "POST",
			path:          "/v2/org/team/app/blobs/uploads/",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "startUpload",
		},
		{
			name:          "Three-level: GET /v2/company/dept/service/manifests/v1.0",
			method:        "GET",
			path:          "/v2/company/dept/service/manifests/v1.0",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "getManifest",
		},

		// Edge cases
		{
			name:          "Path with 'blobs' in repository name",
			method:        "GET",
			path:          "/v2/myblobs/tags/list",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "getTags",
		},
		{
			name:          "Path with 'manifests' in repository name",
			method:        "GET",
			path:          "/v2/app-manifests/tags/list",
			expectStatus:  http.StatusUnauthorized,
			expectHandler: "getTags",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code == http.StatusNotFound {
				t.Errorf("Got 404 Not Found - chi router failed to match route for %s %s", tc.method, tc.path)
				t.Errorf("This indicates the router pattern is not correctly matching the path")
			}

			// We expect Unauthorized (401) because we didn't provide auth token
			// This proves the route matched and middleware ran
			if w.Code != tc.expectStatus {
				t.Errorf("Expected status %d, got %d for %s %s", tc.expectStatus, w.Code, tc.method, tc.path)
			}
		})
	}
}

// TestGetRepoContext verifies repository path extraction logic
func TestGetRepoContext(t *testing.T) {
	testCases := []struct {
		name             string
		urlPath          string
		expectedFullRepo string
		expectedQuotaNs  string
	}{
		{
			name:             "Single-level with blobs endpoint",
			urlPath:          "/v2/tokenworx/blobs/uploads/",
			expectedFullRepo: "tokenworx",
			expectedQuotaNs:  "",
		},
		{
			name:             "Two-level with blobs endpoint",
			urlPath:          "/v2/web3dns/aiserve-farm/blobs/uploads/",
			expectedFullRepo: "web3dns/aiserve-farm",
			expectedQuotaNs:  "web3dns",
		},
		{
			name:             "Three-level with manifests endpoint",
			urlPath:          "/v2/org/team/app/manifests/latest",
			expectedFullRepo: "org/team/app",
			expectedQuotaNs:  "org",
		},
		{
			name:             "Single-level with tags endpoint",
			urlPath:          "/v2/alpine/tags/list",
			expectedFullRepo: "alpine",
			expectedQuotaNs:  "",
		},
		{
			name:             "Two-level with tags endpoint",
			urlPath:          "/v2/acme/myapp/tags/list",
			expectedFullRepo: "acme/myapp",
			expectedQuotaNs:  "acme",
		},
		{
			name:             "Path with 'blobs' in repo name",
			urlPath:          "/v2/myblobs/manifests/v1",
			expectedFullRepo: "myblobs",
			expectedQuotaNs:  "",
		},
		{
			name:             "Five-level repository",
			urlPath:          "/v2/a/b/c/d/e/blobs/sha256:abc123",
			expectedFullRepo: "a/b/c/d/e",
			expectedQuotaNs:  "a",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.urlPath, nil)
			fullRepo, quotaNs := getRepoContext(req)

			if fullRepo != tc.expectedFullRepo {
				t.Errorf("Expected fullRepo=%s, got %s", tc.expectedFullRepo, fullRepo)
			}

			if quotaNs != tc.expectedQuotaNs {
				t.Errorf("Expected quotaNs=%s, got %s", tc.expectedQuotaNs, quotaNs)
			}
		})
	}
}
