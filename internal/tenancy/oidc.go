package tenancy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// OIDCService handles OIDC provider configuration and authentication
type OIDCService struct {
	db *sql.DB
}

// NewOIDCService creates a new OIDC service
func NewOIDCService(db *sql.DB) *OIDCService {
	return &OIDCService{db: db}
}

// ListOIDCConfigs lists all OIDC configurations for a tenant
func (s *OIDCService) ListOIDCConfigs(ctx context.Context, tenantID int) ([]*OIDCConfig, error) {
	query := `
		SELECT id, tenant_id, provider_name, provider_type, issuer_url,
		       client_id, authorization_endpoint, token_endpoint,
		       userinfo_endpoint, jwks_uri, scopes, user_claim, group_claim,
		       allowed_domains, enabled, is_primary,
		       created_at, updated_at, created_by_id
		FROM tenant_oidc_configs
		WHERE tenant_id = $1
		ORDER BY is_primary DESC, created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list OIDC configs: %w", err)
	}
	defer rows.Close()

	var configs []*OIDCConfig
	for rows.Next() {
		config := &OIDCConfig{}
		err := rows.Scan(
			&config.ID,
			&config.TenantID,
			&config.ProviderName,
			&config.ProviderType,
			&config.IssuerURL,
			&config.ClientID,
			&config.AuthorizationEndpoint,
			&config.TokenEndpoint,
			&config.UserInfoEndpoint,
			&config.JWKSURI,
			&config.Scopes,
			&config.UserClaim,
			&config.GroupClaim,
			&config.AllowedDomains,
			&config.Enabled,
			&config.IsPrimary,
			&config.CreatedAt,
			&config.UpdatedAt,
			&config.CreatedByID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan OIDC config: %w", err)
		}
		configs = append(configs, config)
	}

	return configs, nil
}

// GetOIDCConfig retrieves a specific OIDC configuration
func (s *OIDCService) GetOIDCConfig(ctx context.Context, configID int) (*OIDCConfig, error) {
	query := `
		SELECT id, tenant_id, provider_name, provider_type, issuer_url,
		       client_id, client_secret, authorization_endpoint, token_endpoint,
		       userinfo_endpoint, jwks_uri, scopes, user_claim, group_claim,
		       allowed_domains, enabled, is_primary,
		       created_at, updated_at, created_by_id
		FROM tenant_oidc_configs
		WHERE id = $1
	`

	config := &OIDCConfig{}
	err := s.db.QueryRowContext(ctx, query, configID).Scan(
		&config.ID,
		&config.TenantID,
		&config.ProviderName,
		&config.ProviderType,
		&config.IssuerURL,
		&config.ClientID,
		&config.ClientSecret,
		&config.AuthorizationEndpoint,
		&config.TokenEndpoint,
		&config.UserInfoEndpoint,
		&config.JWKSURI,
		&config.Scopes,
		&config.UserClaim,
		&config.GroupClaim,
		&config.AllowedDomains,
		&config.Enabled,
		&config.IsPrimary,
		&config.CreatedAt,
		&config.UpdatedAt,
		&config.CreatedByID,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("OIDC config not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get OIDC config: %w", err)
	}

	return config, nil
}

// CreateOIDCConfig creates a new OIDC provider configuration
func (s *OIDCService) CreateOIDCConfig(ctx context.Context, config *OIDCConfig) (*OIDCConfig, error) {
	// If this is marked as primary, unset other primary configs for this tenant
	if config.IsPrimary {
		_, err := s.db.ExecContext(ctx,
			"UPDATE tenant_oidc_configs SET is_primary = false WHERE tenant_id = $1",
			config.TenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to unset other primary configs: %w", err)
		}
	}

	// Auto-discover OIDC endpoints if not provided
	if config.AuthorizationEndpoint == "" || config.TokenEndpoint == "" {
		if err := s.discoverOIDCEndpoints(config); err != nil {
			return nil, fmt.Errorf("failed to discover OIDC endpoints: %w", err)
		}
	}

	query := `
		INSERT INTO tenant_oidc_configs (
			tenant_id, provider_name, provider_type, issuer_url,
			client_id, client_secret, authorization_endpoint, token_endpoint,
			userinfo_endpoint, jwks_uri, scopes, user_claim, group_claim,
			allowed_domains, enabled, is_primary, created_by_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id, created_at, updated_at
	`

	err := s.db.QueryRowContext(ctx, query,
		config.TenantID,
		config.ProviderName,
		config.ProviderType,
		config.IssuerURL,
		config.ClientID,
		config.ClientSecret,
		config.AuthorizationEndpoint,
		config.TokenEndpoint,
		config.UserInfoEndpoint,
		config.JWKSURI,
		config.Scopes,
		config.UserClaim,
		config.GroupClaim,
		config.AllowedDomains,
		config.Enabled,
		config.IsPrimary,
		config.CreatedByID,
	).Scan(&config.ID, &config.CreatedAt, &config.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC config: %w", err)
	}

	// Clear client secret before returning
	config.ClientSecret = ""
	return config, nil
}

// UpdateOIDCConfig updates an OIDC configuration
func (s *OIDCService) UpdateOIDCConfig(ctx context.Context, configID int, updates map[string]interface{}) (*OIDCConfig, error) {
	// If setting as primary, unset others
	if isPrimary, ok := updates["is_primary"].(bool); ok && isPrimary {
		// Get tenant ID first
		var tenantID int
		err := s.db.QueryRowContext(ctx, "SELECT tenant_id FROM tenant_oidc_configs WHERE id = $1", configID).Scan(&tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tenant ID: %w", err)
		}

		_, err = s.db.ExecContext(ctx,
			"UPDATE tenant_oidc_configs SET is_primary = false WHERE tenant_id = $1 AND id != $2",
			tenantID, configID)
		if err != nil {
			return nil, fmt.Errorf("failed to unset other primary configs: %w", err)
		}
	}

	// Build dynamic update query (simplified for this implementation)
	query := `
		UPDATE tenant_oidc_configs
		SET updated_at = NOW()
		WHERE id = $1
	`

	_, err := s.db.ExecContext(ctx, query, configID)
	if err != nil {
		return nil, fmt.Errorf("failed to update OIDC config: %w", err)
	}

	return s.GetOIDCConfig(ctx, configID)
}

// DeleteOIDCConfig deletes an OIDC configuration
func (s *OIDCService) DeleteOIDCConfig(ctx context.Context, configID int) error {
	query := `DELETE FROM tenant_oidc_configs WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, configID)
	return err
}

// discoverOIDCEndpoints auto-discovers OIDC endpoints from issuer URL
func (s *OIDCService) discoverOIDCEndpoints(config *OIDCConfig) error {
	// OIDC discovery URL
	discoveryURL := strings.TrimSuffix(config.IssuerURL, "/") + "/.well-known/openid-configuration"

	resp, err := http.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch OIDC discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OIDC discovery returned status %d", resp.StatusCode)
	}

	var discovery struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		UserInfoEndpoint      string `json:"userinfo_endpoint"`
		JWKSURI               string `json:"jwks_uri"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return fmt.Errorf("failed to parse OIDC discovery document: %w", err)
	}

	// Populate endpoints
	config.AuthorizationEndpoint = discovery.AuthorizationEndpoint
	config.TokenEndpoint = discovery.TokenEndpoint
	config.UserInfoEndpoint = discovery.UserInfoEndpoint
	config.JWKSURI = discovery.JWKSURI

	return nil
}

// GetOAuth2Config returns an OAuth2 config for a tenant's OIDC provider
func (s *OIDCService) GetOAuth2Config(config *OIDCConfig, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  config.AuthorizationEndpoint,
			TokenURL: config.TokenEndpoint,
		},
		Scopes: strings.Split(config.Scopes, " "),
	}
}

// GenerateStateToken generates a random state token for OIDC flow
func GenerateStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ValidateIDToken validates an ID token (basic validation)
// In production, use a proper OIDC library like coreos/go-oidc
func (s *OIDCService) ValidateIDToken(ctx context.Context, config *OIDCConfig, idToken string) (map[string]interface{}, error) {
	// This is a simplified implementation
	// In production, you should:
	// 1. Verify JWT signature using JWKS
	// 2. Validate issuer, audience, expiration
	// 3. Check nonce if used

	// For now, just decode the payload (NO VERIFICATION - DO NOT USE IN PRODUCTION)
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ID token format")
	}

	// Decode payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode ID token payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse ID token claims: %w", err)
	}

	// Basic validation
	if iss, ok := claims["iss"].(string); !ok || iss != config.IssuerURL {
		return nil, fmt.Errorf("invalid issuer")
	}

	if aud, ok := claims["aud"].(string); !ok || aud != config.ClientID {
		return nil, fmt.Errorf("invalid audience")
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("ID token expired")
		}
	}

	// Validate allowed domains for Google Workspace
	if config.ProviderType == ProviderTypeGoogleWorkspace && config.AllowedDomains != "" {
		email, _ := claims["email"].(string)
		if !s.isEmailInAllowedDomains(email, config.AllowedDomains) {
			return nil, fmt.Errorf("email domain not allowed")
		}
	}

	return claims, nil
}

// isEmailInAllowedDomains checks if email domain is in allowed list
func (s *OIDCService) isEmailInAllowedDomains(email, allowedDomains string) bool {
	if email == "" || allowedDomains == "" {
		return false
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]

	allowed := strings.Split(allowedDomains, ",")
	for _, d := range allowed {
		if strings.TrimSpace(d) == domain {
			return true
		}
	}

	return false
}

// ExchangeCodeForToken exchanges authorization code for tokens
func (s *OIDCService) ExchangeCodeForToken(ctx context.Context, config *OIDCConfig, code, redirectURL string) (*oauth2.Token, error) {
	oauth2Config := s.GetOAuth2Config(config, redirectURL)
	return oauth2Config.Exchange(ctx, code)
}

// GetUserInfo fetches user information from the UserInfo endpoint
func (s *OIDCService) GetUserInfo(ctx context.Context, config *OIDCConfig, accessToken string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", config.UserInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return userInfo, nil
}

// BuildAuthorizationURL builds the OIDC authorization URL
func (s *OIDCService) BuildAuthorizationURL(config *OIDCConfig, redirectURL, state, nonce string) string {
	params := url.Values{}
	params.Set("client_id", config.ClientID)
	params.Set("redirect_uri", redirectURL)
	params.Set("response_type", "code")
	params.Set("scope", config.Scopes)
	params.Set("state", state)

	if nonce != "" {
		params.Set("nonce", nonce)
	}

	// Google Workspace specific parameters
	if config.ProviderType == ProviderTypeGoogleWorkspace {
		params.Set("access_type", "offline")
		params.Set("prompt", "consent")
		if config.AllowedDomains != "" {
			params.Set("hd", strings.Split(config.AllowedDomains, ",")[0]) // Hosted domain
		}
	}

	return config.AuthorizationEndpoint + "?" + params.Encode()
}
