package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	registryAuth "github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"golang.org/x/oauth2"
)

type OIDCHandler struct {
	config       config.OIDCConfig
	db           db.Store
	tokenService *registryAuth.TokenService
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
	states       map[string]time.Time // state -> expiry time (simple in-memory store)
}

func NewOIDCHandler(cfg config.OIDCConfig, dbStore db.Store, ts *registryAuth.TokenService) (*OIDCHandler, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	return &OIDCHandler{
		config:       cfg,
		db:           dbStore,
		tokenService: ts,
		oauth2Config: oauth2Config,
		verifier:     verifier,
		states:       make(map[string]time.Time),
	}, nil
}

func (h *OIDCHandler) Register(mux chi.Router) {
	if h == nil {
		return
	}
	mux.Get("/oauth2/sso", h.handleSSOLogin)
	mux.Get("/oauth2/callback", h.handleCallback)
}

func (h *OIDCHandler) handleSSOLogin(w http.ResponseWriter, r *http.Request) {
	// Generate state token
	state, err := generateRandomString(32)
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	// Store state with expiry (5 minutes)
	h.states[state] = time.Now().Add(5 * time.Minute)

	// Clean up expired states
	h.cleanupExpiredStates()

	// Redirect to Authentik
	authURL := h.oauth2Config.AuthCodeURL(state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *OIDCHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify state
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "Missing state parameter", http.StatusBadRequest)
		return
	}

	expiry, ok := h.states[state]
	if !ok || time.Now().After(expiry) {
		http.Error(w, "Invalid or expired state", http.StatusBadRequest)
		return
	}
	delete(h.states, state)

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		return
	}

	oauth2Token, err := h.oauth2Config.Exchange(ctx, code)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to exchange token: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No id_token in token response", http.StatusInternalServerError)
		return
	}

	// Verify ID token
	idToken, err := h.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to verify ID token: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract claims
	var claims struct {
		Email         string   `json:"email"`
		EmailVerified bool     `json:"email_verified"`
		Name          string   `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Groups        []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse claims: %v", err), http.StatusInternalServerError)
		return
	}

	// Determine username (prefer email, fallback to preferred_username)
	username := claims.Email
	if username == "" {
		username = claims.PreferredUsername
	}
	if username == "" {
		http.Error(w, "No username in ID token claims", http.StatusBadRequest)
		return
	}

	// Determine scopes based on groups
	scopes := []string{}
	isAdmin := false
	for _, group := range claims.Groups {
		if group == "admins" || group == "registry-admins" {
			scopes = []string{"*"}
			isAdmin = true
			break
		}
		// Add group-based scopes (e.g., "repository:groupname/*:pull,push")
		scopes = append(scopes, fmt.Sprintf("repository:%s/*:pull,push", group))
	}

	// Default scope if no groups
	if len(scopes) == 0 {
		scopes = []string{"repository:" + username + "/*:pull,push"}
	}

	// Get or create user
	user, err := h.db.GetUserByUsername(ctx, username)
	if err == db.ErrNotFound {
		// Create new user with empty password (SSO-only)
		if err := h.db.CreateUser(ctx, username, "", scopes); err != nil {
			http.Error(w, fmt.Sprintf("Failed to create user: %v", err), http.StatusInternalServerError)
			return
		}
		user = &db.User{
			Username: username,
			Scopes:   scopes,
		}
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Generate registry JWT token
	access := []registryAuth.AccessEntry{
		{
			Type:    "repository",
			Name:    "*",
			Actions: []string{"*"},
		},
	}

	token, err := h.tokenService.GenerateToken(user.Username, access)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(h.tokenService.Expiration)

	// Redirect to frontend with token in URL fragment (SPA-safe)
	redirectURL := fmt.Sprintf("/#/auth/callback?token=%s&expires_at=%s&username=%s&is_admin=%t",
		token,
		expiresAt.Format(time.RFC3339),
		username,
		isAdmin,
	)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *OIDCHandler) cleanupExpiredStates() {
	now := time.Now()
	for state, expiry := range h.states {
		if now.After(expiry) {
			delete(h.states, state)
		}
	}
}

func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

func extractNamespaces(scopes []string, isAdmin bool) []string {
	if isAdmin {
		return []string{}
	}

	namespaces := []string{}
	for _, scope := range scopes {
		if ns := extractNamespaceFromScope(scope); ns != "" {
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces
}
