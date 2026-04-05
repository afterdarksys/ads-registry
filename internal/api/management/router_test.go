package management

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
)

func TestManagementRouting(t *testing.T) {
	dbStore := &db.MockStore{}
	
	authCfg := config.AuthConfig{
		TokenIssuer:  "test-registry",
		TokenService: "registry",
	}
	tokenService, err := auth.NewTokenService(authCfg)
	if err != nil {
		t.Fatalf("Failed to create token service: %v", err)
	}

	router := NewRouter(dbStore, tokenService, nil, nil, false)
	mux := chi.NewRouter()
	router.Register(mux)

	testCases := []struct {
		name         string
		method       string
		path         string
		expectStatus int
	}{
		// Tags
		{
			name:         "Single-level tags",
			method:       "GET",
			path:         "/api/v1/management/repositories/nginx/tags",
			expectStatus: http.StatusUnauthorized, // Middleware runs, route found
		},
		{
			name:         "Two-level tags",
			method:       "GET",
			path:         "/api/v1/management/repositories/library/nginx/tags",
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "Three-level tags",
			method:       "GET",
			path:         "/api/v1/management/repositories/org/team/project/tags",
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "Five-level tags",
			method:       "GET",
			path:         "/api/v1/management/repositories/a/b/c/d/e/tags",
			expectStatus: http.StatusUnauthorized,
		},

		// Manifests
		{
			name:         "Single-level manifests",
			method:       "GET",
			path:         "/api/v1/management/repositories/nginx/manifests",
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "Two-level manifests",
			method:       "GET",
			path:         "/api/v1/management/repositories/library/nginx/manifests",
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "Three-level manifests",
			method:       "GET",
			path:         "/api/v1/management/repositories/org/team/project/manifests",
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "Five-level manifests",
			method:       "GET",
			path:         "/api/v1/management/repositories/a/b/c/d/e/manifests",
			expectStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code == http.StatusNotFound {
				t.Errorf("Got 404 Not Found - chi router failed to match route for %s %s", tc.method, tc.path)
			}

			if w.Code != tc.expectStatus {
				t.Errorf("Expected status %d, got %d for %s %s", tc.expectStatus, w.Code, tc.method, tc.path)
			}
		})
	}
}
