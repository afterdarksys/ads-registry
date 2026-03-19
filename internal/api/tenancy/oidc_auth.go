package tenancy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/tenancy"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

// OIDCAuthHandler handles OIDC authentication flow
type OIDCAuthHandler struct {
	oidcService   *tenancy.OIDCService
	tenantService *tenancy.TenantService
	store         db.Store
	tokenService  *auth.TokenService
	sessionStore  sessions.Store
	baseURL       string
}

// NewOIDCAuthHandler creates a new OIDC auth handler
func NewOIDCAuthHandler(database *sql.DB, store db.Store, tokenService *auth.TokenService, baseURL string) *OIDCAuthHandler {
	// Create session store (use secure cookie store in production)
	sessionStore := sessions.NewCookieStore([]byte("your-secret-key-change-in-production"))

	return &OIDCAuthHandler{
		oidcService:   tenancy.NewOIDCService(database),
		tenantService: tenancy.NewTenantService(database),
		store:         store,
		tokenService:  tokenService,
		sessionStore:  sessionStore,
		baseURL:       baseURL,
	}
}

// RegisterRoutes registers OIDC authentication routes
func (h *OIDCAuthHandler) RegisterRoutes(router chi.Router) {
	// OIDC authentication flow
	router.Get("/auth/oidc/login", h.initiateOIDCLogin)
	router.Get("/auth/oidc/callback", h.handleOIDCCallback)
	router.HandleFunc("/auth/oidc/logout", h.handleOIDCLogout) // Both GET and POST
}

// initiateOIDCLogin starts the OIDC authentication flow
func (h *OIDCAuthHandler) initiateOIDCLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get tenant from context
	tenant, ok := tenancy.GetTenantFromContext(ctx)
	if !ok {
		http.Error(w, "Tenant not found", http.StatusBadRequest)
		return
	}

	// Get OIDC configs for tenant
	configs, err := h.oidcService.ListOIDCConfigs(ctx, tenant.ID)
	if err != nil || len(configs) == 0 {
		http.Error(w, "No OIDC provider configured", http.StatusBadRequest)
		return
	}

	// Use primary config or first enabled config
	var config *tenancy.OIDCConfig
	for _, c := range configs {
		if c.Enabled && (c.IsPrimary || config == nil) {
			config = c
			if c.IsPrimary {
				break
			}
		}
	}

	if config == nil {
		http.Error(w, "No enabled OIDC provider found", http.StatusBadRequest)
		return
	}

	// Generate state token for CSRF protection
	state, err := tenancy.GenerateStateToken()
	if err != nil {
		http.Error(w, "Failed to generate state token", http.StatusInternalServerError)
		return
	}

	// Generate nonce for replay protection
	nonce, err := tenancy.GenerateStateToken()
	if err != nil {
		http.Error(w, "Failed to generate nonce", http.StatusInternalServerError)
		return
	}

	// Store state and nonce in session
	session, _ := h.sessionStore.Get(r, "oidc-session")
	session.Values["state"] = state
	session.Values["nonce"] = nonce
	session.Values["tenant_id"] = tenant.ID
	session.Values["config_id"] = config.ID
	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
	}

	// Build redirect URL
	redirectURL := fmt.Sprintf("%s/auth/oidc/callback", h.baseURL)

	// Build authorization URL
	authURL := h.oidcService.BuildAuthorizationURL(config, redirectURL, state, nonce)

	// Redirect to OIDC provider
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCCallback handles the OIDC callback after authentication
func (h *OIDCAuthHandler) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get session
	session, err := h.sessionStore.Get(r, "oidc-session")
	if err != nil {
		http.Error(w, "Invalid session", http.StatusBadRequest)
		return
	}

	// Verify state parameter (CSRF protection)
	storedState, _ := session.Values["state"].(string)
	receivedState := r.URL.Query().Get("state")
	if storedState == "" || storedState != receivedState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Get tenant and config from session
	tenantID, _ := session.Values["tenant_id"].(int)
	configID, _ := session.Values["config_id"].(int)

	// Get OIDC config
	config, err := h.oidcService.GetOIDCConfig(ctx, configID)
	if err != nil {
		http.Error(w, "OIDC configuration not found", http.StatusInternalServerError)
		return
	}

	// Exchange authorization code for tokens
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	redirectURL := fmt.Sprintf("%s/auth/oidc/callback", h.baseURL)
	token, err := h.oidcService.ExchangeCodeForToken(ctx, config, code, redirectURL)
	if err != nil {
		log.Printf("Failed to exchange code for token: %v", err)
		http.Error(w, "Failed to exchange code for token", http.StatusInternalServerError)
		return
	}

	// Extract ID token
	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No ID token returned", http.StatusInternalServerError)
		return
	}

	// Validate ID token
	claims, err := h.oidcService.ValidateIDToken(ctx, config, idToken)
	if err != nil {
		log.Printf("Failed to validate ID token: %v", err)
		http.Error(w, "Invalid ID token", http.StatusUnauthorized)
		return
	}

	// Extract username from configured claim
	username, ok := claims[config.UserClaim].(string)
	if !ok {
		http.Error(w, "Username claim not found in ID token", http.StatusInternalServerError)
		return
	}

	// Get or create user in tenant schema
	user, err := h.getOrCreateUserFromOIDC(ctx, tenantID, username, claims, config)
	if err != nil {
		log.Printf("Failed to get/create user: %v", err)
		http.Error(w, "Failed to authenticate user", http.StatusInternalServerError)
		return
	}

	// Generate JWT for registry access
	// Build access entries from user scopes
	var accessEntries []auth.AccessEntry
	if len(user.Scopes) > 0 {
		// Use user's scopes
		for range user.Scopes {
			accessEntries = append(accessEntries, auth.AccessEntry{
				Type:    "repository",
				Name:    "*",
				Actions: []string{"pull", "push"},
			})
		}
	} else {
		// Default access for OIDC users
		accessEntries = []auth.AccessEntry{
			{
				Type:    "repository",
				Name:    "*",
				Actions: []string{"pull", "push"},
			},
		}
	}

	registryToken, err := h.tokenService.GenerateToken(username, accessEntries)
	if err != nil {
		log.Printf("Failed to issue token: %v", err)
		http.Error(w, "Failed to issue token", http.StatusInternalServerError)
		return
	}

	// Store token in session or cookie
	session.Values["access_token"] = registryToken
	session.Values["username"] = username
	session.Values["user_id"] = user.ID
	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
	}

	// Return success response with token
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":    registryToken,
		"username": username,
		"expires":  time.Now().Add(24 * time.Hour).Unix(),
	})
}

// handleOIDCLogout handles OIDC logout
func (h *OIDCAuthHandler) handleOIDCLogout(w http.ResponseWriter, r *http.Request) {
	// Get session
	session, err := h.sessionStore.Get(r, "oidc-session")
	if err == nil {
		// Clear session
		session.Options.MaxAge = -1
		session.Save(r, w)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Logged out successfully",
	})
}

// getOrCreateUserFromOIDC gets or creates a user from OIDC claims
func (h *OIDCAuthHandler) getOrCreateUserFromOIDC(ctx context.Context, tenantID int, username string, claims map[string]interface{}, config *tenancy.OIDCConfig) (*db.User, error) {
	// Try to get existing user directly from store
	user, err := h.store.GetUserByUsername(ctx, username)
	if err == nil {
		// User exists
		return user, nil
	}

	// User doesn't exist, create new user
	// Default scopes for OIDC users
	scopes := []string{"repository:*:pull", "repository:*:push"}

	// Create unusable password hash for OIDC users
	unusableHash := "$2a$10$OIDC.USER.NO.PASSWORD.REQUIRED"

	err = h.store.CreateUser(ctx, username, unusableHash, scopes)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Retrieve the created user
	user, err = h.store.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve created user: %w", err)
	}

	return user, nil
}

// Removed getUserByUsername and createUser - using db.Store methods directly
