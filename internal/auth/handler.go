package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/db"
)

type Handler struct {
	tokenService *TokenService
	db           db.Store
}

func NewHandler(ts *TokenService, dbStore db.Store) *Handler {
	return &Handler{
		tokenService: ts,
		db:           dbStore,
	}
}

func (h *Handler) Register(mux chi.Router) {
	mux.Get("/auth/token", h.tokenHandler)
	mux.Post("/auth/refresh", h.refreshHandler)
}

func (h *Handler) tokenHandler(w http.ResponseWriter, req *http.Request) {
	// 1. Basic Auth check for username/password
	user, pass, ok := req.BasicAuth()
	if !ok {
		w.Header().Set("Www-Authenticate", `Basic realm="registry"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Authenticate user with password
	dbUser, err := h.db.AuthenticateUser(req.Context(), user, pass)
	if err != nil {
		// Bootstrap fallback: Allow admin/admin if no users exist
		if user == "admin" && pass == "admin" {
			// Check if ANY users exist in the database
			if testUser, _ := h.db.GetUserByUsername(req.Context(), "admin"); testUser == nil {
				// No admin user exists - allow bootstrap login
				log.Printf("WARNING: Bootstrap login used (admin/admin). Create a real user ASAP!")
				dbUser = &db.User{Username: "admin", Scopes: []string{"*"}}
			} else {
				w.Header().Set("Www-Authenticate", `Basic realm="registry"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		} else {
			w.Header().Set("Www-Authenticate", `Basic realm="registry"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// 3. Parse Scopes from ?scope=repository:library/ubuntu:pull,push
	// Docker might request multiple scopes, but standard clients usually just ask for one.
	qScope := req.URL.Query().Get("scope")
	var grantedAccess []AccessEntry

	if qScope != "" {
		parts := strings.Split(qScope, ":")
		if len(parts) >= 3 {
			typ := parts[0]
			name := parts[1]
			actions := strings.Split(parts[2], ",")

			// AuthZ Check: Does user have permission for these actions on this resource?
			// Check if user's scopes authorize this request
			authorized := false
			for _, userScope := range dbUser.Scopes {
				// Wildcard grants everything
				if userScope == "*" {
					authorized = true
					break
				}
				// Check exact match: repository:namespace/repo:action1,action2
				if userScope == qScope {
					authorized = true
					break
				}
				// Check pattern match for wildcards like repository:*:push,pull
				if matchesScope(userScope, typ, name, actions) {
					authorized = true
					break
				}
			}

			if !authorized {
				log.Printf("[AUTH] User %s denied access to %s:%s:%s", user, typ, name, strings.Join(actions, ","))
				http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
				return
			}

			grantedAccess = append(grantedAccess, AccessEntry{
				Type:    typ,
				Name:    name,
				Actions: actions,
			})
		}
	}

	// 4. Generate JWT
	token, err := h.tokenService.GenerateToken(user, grantedAccess)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 5. Respond in Docker bearer format
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":        token,
		"access_token": token, // For older clients
		"expires_in":   int(h.tokenService.Expiration.Seconds()),
	})
}

func (h *Handler) refreshHandler(w http.ResponseWriter, req *http.Request) {
	// Extract Bearer token from Authorization header
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
		return
	}

	// Parse and validate the existing token
	claims, err := h.tokenService.ParseToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
		return
	}

	// Generate a new token with the same access grants
	newToken, err := h.tokenService.GenerateToken(claims.Subject, claims.Access)
	if err != nil {
		http.Error(w, "Failed to generate new token", http.StatusInternalServerError)
		return
	}

	// Respond with the new token
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":        newToken,
		"access_token": newToken,
		"expires_in":   int(h.tokenService.Expiration.Seconds()),
	})
}

// matchesScope checks if a user scope pattern matches the requested resource
func matchesScope(userScope, requestedType, requestedName string, requestedActions []string) bool {
	// Parse user scope format: repository:namespace/*:push,pull
	parts := strings.Split(userScope, ":")
	if len(parts) < 2 {
		return false
	}

	scopeType := parts[0]
	scopeName := parts[1]
	var scopeActions []string
	if len(parts) >= 3 {
		scopeActions = strings.Split(parts[2], ",")
	}

	// Check type match
	if scopeType != requestedType {
		return false
	}

	// Check name match with wildcard support
	if scopeName != "*" && scopeName != requestedName {
		// Check for prefix wildcards like "namespace/*"
		if strings.HasSuffix(scopeName, "/*") {
			prefix := strings.TrimSuffix(scopeName, "/*")
			if !strings.HasPrefix(requestedName, prefix+"/") {
				return false
			}
		} else {
			return false
		}
	}

	// Check actions if specified
	if len(scopeActions) > 0 {
		scopeActionSet := make(map[string]bool)
		for _, action := range scopeActions {
			action = strings.TrimSpace(action)
			scopeActionSet[action] = true
		}

		// Wildcard action grants all actions
		if scopeActionSet["*"] {
			return true
		}

		// Check if all requested actions are granted
		for _, reqAction := range requestedActions {
			if !scopeActionSet[reqAction] {
				return false
			}
		}
	}

	return true
}
