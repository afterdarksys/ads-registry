package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/ryan/ads-registry/internal/logger"
)

type userContextKey string

var UserContext = userContextKey("user")

// CredentialValidator validates a username/password and returns the user's scopes.
// Used to support Basic auth in addition to bearer tokens.
type CredentialValidator func(ctx context.Context, username, password string) ([]string, error)

type Middleware struct {
	tokenService        *TokenService
	serviceName         string
	developerMode       bool
	credentialValidator CredentialValidator
}

func NewMiddleware(ts *TokenService, developerMode bool, validator CredentialValidator) *Middleware {
	return &Middleware{
		tokenService:        ts,
		serviceName:         ts.service,
		developerMode:       developerMode,
		credentialValidator: validator,
	}
}

// GetScheme determines the protocol scheme (http/https) from the request
func GetScheme(r *http.Request) string {
	log.Printf("[AUTH/getScheme] Called with Host=%s, TLS=%v, X-Forwarded-Proto=%s",
		r.Host, r.TLS != nil, r.Header.Get("X-Forwarded-Proto"))

	// Force HTTP for localhost to avoid TLS issues
	if strings.HasPrefix(r.Host, "localhost:") || strings.HasPrefix(r.Host, "127.0.0.1:") {
		log.Printf("[AUTH] Forcing HTTP for localhost: Host=%s", r.Host)
		return "http"
	}

	// Check if request came via TLS
	if r.TLS != nil {
		log.Printf("[AUTH] Detected HTTPS via r.TLS: Host=%s", r.Host)
		return "https"
	}

	// Check X-Forwarded-Proto header (set by reverse proxies)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		log.Printf("[AUTH] Using X-Forwarded-Proto=%s: Host=%s", proto, r.Host)
		return proto
	}

	// Default to http
	log.Printf("[AUTH] Defaulting to HTTP: Host=%s", r.Host)
	return "http"
}

func (m *Middleware) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme := GetScheme(r)

		if m.developerMode {
			logger.Debug("DeveloperMode bypass active in Protect router")
			next.ServeHTTP(w, r)
			return
		}

		// Bypass auth challenge if hitting /v2/ root without token
		if r.URL.Path == "/v2/" && r.Header.Get("Authorization") == "" {
			// Instruct docker client to get a token
			w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s://%s/auth/token",service="%s"`, scheme, r.Host, m.serviceName))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		authHeader := r.Header.Get("Authorization")

		// Basic auth path — containerd (k3s ≤1.34) cannot complete the bearer
		// token exchange, so we validate credentials inline and proceed directly.
		if strings.HasPrefix(authHeader, "Basic ") && m.credentialValidator != nil {
			username, password, ok := parseBasicAuth(authHeader)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			scopes, err := m.credentialValidator(r.Context(), username, password)
			if err != nil {
				log.Printf("[AUTH] Basic auth failed for %s: %v", username, err)
				w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s://%s/auth/token",service="%s"`, scheme, r.Host, m.serviceName))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			action := getRequiredAction(r)
			fullRepo := extractRepo(r.URL.Path)

			authorized := r.URL.Path == "/v2/" || r.URL.Path == "/v2/_catalog"
			if !authorized {
				for _, s := range scopes {
					if s == "*" || matchesScope(s, "repository", fullRepo, []string{action}) {
						authorized = true
						break
					}
				}
			}

			if !authorized {
				log.Printf("[AUTH] Basic auth denied: user=%s repo=%s action=%s", username, fullRepo, action)
				http.Error(w, `{"errors":[{"code":"DENIED","message":"requested access to the resource is denied"}]}`, http.StatusForbidden)
				return
			}

			claims := Claims{
				Access: []AccessEntry{{Type: "repository", Name: fullRepo, Actions: []string{action}}},
			}
			claims.Subject = username
			ctx := context.WithValue(r.Context(), UserContext, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			// Require auth
			w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s://%s/auth/token",service="%s"`, scheme, r.Host, m.serviceName))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims, err := m.tokenService.ParseToken(tokenString)
		if err != nil {
			http.Error(w, `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`, http.StatusUnauthorized)
			return
		}

		// Validation of action -> scope
		action := getRequiredAction(r)

		// Extract repository path directly from URL (same approach as handlers)
		// Strip endpoint suffixes like /blobs/, /manifests/, /tags/, /referrers/
		endpointPrefixes := []string{
			"/blobs/",
			"/manifests/",
			"/tags/",
			"/referrers/",
		}

		cleanPath := r.URL.Path
		for _, prefix := range endpointPrefixes {
			if idx := strings.Index(cleanPath, prefix); idx != -1 {
				cleanPath = cleanPath[:idx]
				break
			}
		}

		// Remove /v2/ prefix to get full repository path
		fullRepo := strings.TrimPrefix(cleanPath, "/v2/")
		if fullRepo == "" || fullRepo == "/" {
			fullRepo = "*"
		}

		logger.Debug("Checking authorization: URL=%s fullRepo=%s action=%s", r.URL.Path, fullRepo, action)

		authorized := false
		for _, access := range claims.Access {
			logger.Debug("JWT Access: type=%s name=%s actions=%v", access.Type, access.Name, access.Actions)
			
			// Allow catalog access
			if r.URL.Path == "/v2/_catalog" && access.Type == "registry" && access.Name == "catalog" {
				for _, allowedAction := range access.Actions {
					if allowedAction == "*" {
						authorized = true
						break
					}
				}
			}

			if access.Type == "repository" && access.Name == fullRepo {
				for _, allowedAction := range access.Actions {
					if allowedAction == action || allowedAction == "*" {
						authorized = true
						break
					}
				}
			}
		}

		if !authorized {
			logger.Debug("Authorization DENIED: fullRepo=%s from JWT != expected or action %s not in token", fullRepo, action)
		}

		// For the base check `/v2/` we only need a valid token.
		if r.URL.Path == "/v2/" {
			authorized = true
		}

		if !authorized {
			http.Error(w, `{"errors":[{"code":"DENIED","message":"requested access to the resource is denied"}]}`, http.StatusForbidden)
			return
		}

		// Set context and continue
		ctx := context.WithValue(r.Context(), UserContext, *claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) ProtectAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.developerMode {
			logger.Debug("DeveloperMode bypass active in ProtectAdmin router")
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized: missing or invalid token", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := m.tokenService.ParseToken(tokenString)
		if err != nil {
			http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
			return
		}

		// Check if user has admin scope (wildcard "*")
		isAdmin := false
		for _, access := range claims.Access {
			// Admin is defined as having wildcard scope
			if access.Type == "repository" && access.Name == "*" {
				for _, action := range access.Actions {
					if action == "*" {
						isAdmin = true
						break
					}
				}
			}
			// Also check for direct wildcard in access
			for _, action := range access.Actions {
				if action == "*" && access.Name == "*" {
					isAdmin = true
					break
				}
			}
		}

		if !isAdmin {
			logger.Warn("Access denied for user %s - requires admin privileges", claims.Subject)
			http.Error(w, "Forbidden: admin privileges required", http.StatusForbidden)
			return
		}

		logger.Debug("Admin access granted for user %s", claims.Subject)

		// Set context and continue
		ctx := context.WithValue(r.Context(), UserContext, *claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getRequiredAction(r *http.Request) string {
	switch r.Method {
	case "GET", "HEAD":
		return "pull"
	case "POST", "PUT", "PATCH", "DELETE":
		return "push"
	default:
		return "pull"
	}
}

// extractRepo strips /v2/ prefix and endpoint suffixes to get the repository path.
func extractRepo(urlPath string) string {
	for _, suffix := range []string{"/blobs/", "/manifests/", "/tags/", "/referrers/"} {
		if idx := strings.Index(urlPath, suffix); idx != -1 {
			urlPath = urlPath[:idx]
			break
		}
	}
	repo := strings.TrimPrefix(urlPath, "/v2/")
	if repo == "" || repo == "/" {
		return "*"
	}
	return repo
}

// parseBasicAuth decodes an "Authorization: Basic <base64>" header value.
func parseBasicAuth(header string) (username, password string, ok bool) {
	encoded := strings.TrimPrefix(header, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
