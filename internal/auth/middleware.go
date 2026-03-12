package auth

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type userContextKey string

var UserContext = userContextKey("user")

type Middleware struct {
	tokenService *TokenService
}

func NewMiddleware(ts *TokenService) *Middleware {
	return &Middleware{tokenService: ts}
}

func (m *Middleware) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Bypass auth challenge if hitting /v2/ root without token
		if r.URL.Path == "/v2/" && r.Header.Get("Authorization") == "" {
			// Instruct docker client to get a token
			w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="http://%s/auth/token",service="%s"`, r.Host, "registry"))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			// Require auth
			w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="http://%s/auth/token",service="%s"`, r.Host, "registry"))
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
		repo := chi.URLParam(r, "repo")
		ns := chi.URLParam(r, "namespace")
		fullRepo := ns + "/" + repo

		log.Printf("[MIDDLEWARE] Checking authorization: URL=%s ns=%s repo=%s fullRepo=%s action=%s", r.URL.Path, ns, repo, fullRepo, action)

		authorized := false
		for _, access := range claims.Access {
			log.Printf("[MIDDLEWARE] JWT Access: type=%s name=%s actions=%v", access.Type, access.Name, access.Actions)
			
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
			log.Printf("[MIDDLEWARE] Authorization DENIED: fullRepo=%s from JWT != expected or action %s not in token", fullRepo, action)
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
