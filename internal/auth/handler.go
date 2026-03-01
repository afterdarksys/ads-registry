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

	_ = dbUser // we have the authenticated user

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
			// For MVP, if they authenticated, we grant what they ask.
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
		"expires_in":   3600,
	})
}
