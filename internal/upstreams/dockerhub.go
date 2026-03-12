package upstreams

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DockerHubProvider handles Docker Hub authentication
type DockerHubProvider struct {
	client *http.Client
}

// NewDockerHubProvider creates a new Docker Hub provider
func NewDockerHubProvider() *DockerHubProvider {
	return &DockerHubProvider{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the provider type
func (p *DockerHubProvider) Name() UpstreamType {
	return UpstreamTypeDocker
}

// RefreshToken fetches a JWT token from Docker Hub
// Docker Hub tokens are typically short-lived (minutes to hours depending on the endpoint)
func (p *DockerHubProvider) RefreshToken(ctx context.Context, registry *UpstreamRegistry) (token string, expiry time.Time, err error) {
	// Docker Hub has two auth methods:
	// 1. Username/password for docker login (stores credentials, no expiry)
	// 2. JWT tokens from auth.docker.io (for API access, short-lived)

	// For pull/push operations, we can use basic auth with username:password
	// But for better security and Docker Hub API rate limits, we'll get a JWT token

	username := registry.AccessKeyID       // Docker Hub username
	password := registry.SecretAccessKey   // Docker Hub password or access token

	// Request a JWT token from Docker Hub auth service
	// This is used for registry v2 API access
	authURL := "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/alpine:pull"

	req, err := http.NewRequestWithContext(ctx, "GET", authURL, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Set basic auth
	req.SetBasicAuth(username, password)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("auth failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		IssuedAt    string `json:"issued_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse auth response: %w", err)
	}

	// Use the token field, fallback to access_token
	finalToken := authResp.Token
	if finalToken == "" {
		finalToken = authResp.AccessToken
	}

	if finalToken == "" {
		// Fallback: just use basic auth credentials (username:password base64 encoded)
		// This works for Docker Hub but isn't as elegant
		finalToken = base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	}

	// Calculate expiry
	var expiryTime time.Time
	if authResp.ExpiresIn > 0 {
		expiryTime = time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second)
	} else {
		// Default to 1 hour if not specified
		expiryTime = time.Now().Add(1 * time.Hour)
	}

	return finalToken, expiryTime, nil
}

// ValidateCredentials checks if Docker Hub credentials are valid
func (p *DockerHubProvider) ValidateCredentials(ctx context.Context, registry *UpstreamRegistry) error {
	if registry.AccessKeyID == "" {
		return fmt.Errorf("Docker Hub username is required")
	}
	if registry.SecretAccessKey == "" {
		return fmt.Errorf("Docker Hub password/token is required")
	}

	// Try to get a token to validate credentials
	_, _, err := p.RefreshToken(ctx, registry)
	return err
}

// GetRegistryEndpoint returns the Docker Hub registry URL
func (p *DockerHubProvider) GetRegistryEndpoint(registry *UpstreamRegistry, repository string) string {
	// Docker Hub format:
	// - Official images: registry-1.docker.io/library/{image}
	// - User images: registry-1.docker.io/{username}/{image}

	// If repository doesn't have a slash, it's an official image
	if !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	return fmt.Sprintf("registry-1.docker.io/%s", repository)
}

// NeedsRefresh returns true if the token should be refreshed
// Docker Hub tokens are short-lived, refresh 10 minutes before expiry
func (p *DockerHubProvider) NeedsRefresh(registry *UpstreamRegistry) bool {
	if registry.CurrentToken == "" {
		return true
	}
	// Refresh 10 minutes before expiry
	return time.Now().Add(10 * time.Minute).After(registry.TokenExpiry)
}
