package upstreams

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/oauth2/google"
)

// GCPProvider handles Google Cloud Platform Artifact Registry/GCR authentication
type GCPProvider struct{}

// NewGCPProvider creates a new GCP provider
func NewGCPProvider() *GCPProvider {
	return &GCPProvider{}
}

// Name returns the provider type
func (p *GCPProvider) Name() UpstreamType {
	return UpstreamTypeGCP
}

// RefreshToken fetches a new OAuth2 access token from GCP
// GCP tokens are typically valid for 1 hour
func (p *GCPProvider) RefreshToken(ctx context.Context, registry *UpstreamRegistry) (token string, expiry time.Time, err error) {
	// Parse the service account JSON key
	// The key is stored in SecretAccessKey field
	credentials, err := google.CredentialsFromJSON(
		ctx,
		[]byte(registry.SecretAccessKey),
		"https://www.googleapis.com/auth/cloud-platform",
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse GCP credentials: %w", err)
	}

	// Get OAuth2 token
	tokenSource := credentials.TokenSource
	oauthToken, err := tokenSource.Token()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get GCP token: %w", err)
	}

	// GCP uses OAuth2 access tokens
	// For docker login, the format is oauth2accesstoken:<token>
	// But we'll store just the token and prefix it during use

	expiryTime := oauthToken.Expiry
	if expiryTime.IsZero() {
		// Fallback: assume 1 hour if not specified
		expiryTime = time.Now().Add(1 * time.Hour)
	}

	return oauthToken.AccessToken, expiryTime, nil
}

// ValidateCredentials checks if the GCP credentials are valid
func (p *GCPProvider) ValidateCredentials(ctx context.Context, registry *UpstreamRegistry) error {
	if registry.SecretAccessKey == "" {
		return fmt.Errorf("GCP service account JSON is required")
	}
	if registry.Region == "" {
		return fmt.Errorf("GCP region is required (e.g., us-central1)")
	}

	// Validate JSON format and extract project ID
	var serviceAccount struct {
		Type                    string `json:"type"`
		ProjectID               string `json:"project_id"`
		PrivateKeyID            string `json:"private_key_id"`
		PrivateKey              string `json:"private_key"`
		ClientEmail             string `json:"client_email"`
		ClientID                string `json:"client_id"`
		AuthURI                 string `json:"auth_uri"`
		TokenURI                string `json:"token_uri"`
		AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
		ClientX509CertURL       string `json:"client_x509_cert_url"`
	}

	if err := json.Unmarshal([]byte(registry.SecretAccessKey), &serviceAccount); err != nil {
		return fmt.Errorf("invalid GCP service account JSON: %w", err)
	}

	if serviceAccount.Type != "service_account" {
		return fmt.Errorf("invalid GCP credential type: %s (expected service_account)", serviceAccount.Type)
	}

	if serviceAccount.ProjectID == "" {
		return fmt.Errorf("GCP service account missing project_id")
	}

	// Store project ID in AccessKeyID for later use
	if registry.AccessKeyID == "" {
		registry.AccessKeyID = serviceAccount.ProjectID
	}

	// Try to get a token to validate credentials
	_, _, err := p.RefreshToken(ctx, registry)
	return err
}

// GetRegistryEndpoint returns the GCP Artifact Registry or GCR URL
func (p *GCPProvider) GetRegistryEndpoint(registry *UpstreamRegistry, repository string) string {
	// GCP has two formats:
	// 1. Old GCR: gcr.io/{project-id}/{repository}
	// 2. Artifact Registry: {region}-docker.pkg.dev/{project-id}/{repo-name}/{image}

	// Default to Artifact Registry format
	// Format: {region}-docker.pkg.dev/{project}/{repository}
	return fmt.Sprintf("%s/%s", registry.Endpoint, repository)
}

// NeedsRefresh returns true if the token should be refreshed
// GCP tokens are 1 hour, refresh 10 minutes before expiry
func (p *GCPProvider) NeedsRefresh(registry *UpstreamRegistry) bool {
	if registry.CurrentToken == "" {
		return true
	}
	return time.Now().Add(10 * time.Minute).After(registry.TokenExpiry)
}

// GCPAuthToken formats the token for Docker authentication
// GCP requires "oauth2accesstoken" as the username
func GCPAuthToken(token string) (username, password string) {
	return "oauth2accesstoken", token
}

// GCPDockerAuth returns base64-encoded auth for Docker config
func GCPDockerAuth(token string) string {
	username, password := GCPAuthToken(token)
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
