package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/db"
)

type OAuth2Router struct {
	db           db.Store
	tokenService *auth.TokenService
}

func NewOAuth2Router(dbStore db.Store, ts *auth.TokenService) *OAuth2Router {
	return &OAuth2Router{
		db:           dbStore,
		tokenService: ts,
	}
}

func (r *OAuth2Router) Register(mux chi.Router) {
	mux.Post("/oauth2/login", r.handleLogin)
	mux.Get("/oauth2/me", r.handleMe)
	mux.Post("/oauth2/logout", r.handleLogout)
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      UserInfo  `json:"user"`
}

type UserInfo struct {
	Username   string   `json:"username"`
	Scopes     []string `json:"scopes"`
	IsAdmin    bool     `json:"is_admin"`
	Namespaces []string `json:"namespaces"`
}

func (r *OAuth2Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	var loginReq LoginRequest
	if err := json.NewDecoder(req.Body).Decode(&loginReq); err != nil {
		http.Error(w, `{"error":"invalid_request","error_description":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if loginReq.Username == "" || loginReq.Password == "" {
		http.Error(w, `{"error":"invalid_request","error_description":"Username and password required"}`, http.StatusBadRequest)
		return
	}

	// Authenticate user
	user, err := r.db.AuthenticateUser(req.Context(), loginReq.Username, loginReq.Password)
	if err != nil {
		http.Error(w, `{"error":"invalid_grant","error_description":"Invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Determine if user is admin (has wildcard scope)
	isAdmin := false
	for _, scope := range user.Scopes {
		if scope == "*" {
			isAdmin = true
			break
		}
	}

	// Get user's namespaces (for non-admin users)
	namespaces := []string{}
	if !isAdmin {
		// Extract namespaces from scopes (e.g., "repository:myorg/*:pull,push")
		for _, scope := range user.Scopes {
			// Parse scope format: repository:namespace/*:actions
			// This is a simplified version - you may need more complex parsing
			namespaces = append(namespaces, extractNamespaceFromScope(scope))
		}
	}

	// Generate JWT token for UI session
	access := []auth.AccessEntry{
		{
			Type:    "repository",
			Name:    "*",
			Actions: []string{"*"},
		},
	}

	token, err := r.tokenService.GenerateToken(user.Username, access)
	if err != nil {
		http.Error(w, `{"error":"server_error","error_description":"Failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(r.tokenService.Expiration)

	response := LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User: UserInfo{
			Username:   user.Username,
			Scopes:     user.Scopes,
			IsAdmin:    isAdmin,
			Namespaces: namespaces,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (r *OAuth2Router) handleMe(w http.ResponseWriter, req *http.Request) {
	// Extract token from Authorization header
	tokenString := extractBearerToken(req)
	if tokenString == "" {
		http.Error(w, `{"error":"unauthorized","error_description":"Missing or invalid token"}`, http.StatusUnauthorized)
		return
	}

	// Parse and validate token
	claims, err := r.tokenService.ParseToken(tokenString)
	if err != nil {
		http.Error(w, `{"error":"unauthorized","error_description":"Invalid token"}`, http.StatusUnauthorized)
		return
	}

	// Get user details
	user, err := r.db.GetUserByUsername(req.Context(), claims.Subject)
	if err != nil {
		http.Error(w, `{"error":"not_found","error_description":"User not found"}`, http.StatusNotFound)
		return
	}

	// Determine if user is admin
	isAdmin := false
	for _, scope := range user.Scopes {
		if scope == "*" {
			isAdmin = true
			break
		}
	}

	// Get user's namespaces
	namespaces := []string{}
	if !isAdmin {
		for _, scope := range user.Scopes {
			namespaces = append(namespaces, extractNamespaceFromScope(scope))
		}
	}

	userInfo := UserInfo{
		Username:   user.Username,
		Scopes:     user.Scopes,
		IsAdmin:    isAdmin,
		Namespaces: namespaces,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userInfo)
}

func (r *OAuth2Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	// For stateless JWT, logout is handled client-side by removing the token
	// Server-side logout would require token blacklisting (not implemented here)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Logged out successfully",
	})
}

// Helper functions
func extractBearerToken(req *http.Request) string {
	authHeader := req.Header.Get("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}
	return ""
}

func extractNamespaceFromScope(scope string) string {
	// Simple namespace extraction from scope format: repository:namespace/*:actions
	// Example: "repository:myorg/*:pull,push" -> "myorg"
	if len(scope) < 11 || scope[:11] != "repository:" {
		return ""
	}

	remaining := scope[11:]
	// Find the first colon or slash
	for i, ch := range remaining {
		if ch == ':' || ch == '/' {
			if i > 0 {
				return remaining[:i]
			}
			return ""
		}
	}

	return remaining
}
