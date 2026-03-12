package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Kubernetes Kubelet Credential Provider for ADS Container Registry
// Implements: https://kubernetes.io/docs/tasks/administer-cluster/kubelet-credential-provider/

const (
	apiVersion = "credentialprovider.kubelet.k8s.io/v1"
)

// CredentialProviderRequest is the input from kubelet
type CredentialProviderRequest struct {
	Image string `json:"image"`
}

// CredentialProviderResponse is the output to kubelet
type CredentialProviderResponse struct {
	APIVersion string               `json:"apiVersion"`
	Kind       string               `json:"kind"`
	CacheKeyType string             `json:"cacheKeyType"`
	CacheDuration *Duration         `json:"cacheDuration,omitempty"`
	Auth       map[string]AuthConfig `json:"auth"`
}

// AuthConfig contains credentials for a registry
type AuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Duration wraps time.Duration for JSON marshaling
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		return err
	default:
		return fmt.Errorf("invalid duration")
	}
}

// Config holds the provider configuration
type Config struct {
	RegistryURL  string `json:"registry_url"`
	TokenURL     string `json:"token_url"`
	TokenFile    string `json:"token_file"`
	CacheDuration string `json:"cache_duration"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("ads-registry-credential-provider v1.0.0")
		fmt.Println("API Version:", apiVersion)
		os.Exit(0)
	}

	// Read configuration
	config, err := loadConfig()
	if err != nil {
		fatal("Failed to load config: %v", err)
	}

	// Read request from stdin (kubelet sends JSON request)
	var request CredentialProviderRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fatal("Failed to decode request: %v", err)
	}

	debug("Received credential request for image: %s", request.Image)

	// Extract registry from image name
	registry := extractRegistry(request.Image)

	// Check if this is our registry
	if !matchesRegistry(registry, config.RegistryURL) {
		debug("Image %s does not match registry %s, returning empty credentials", request.Image, config.RegistryURL)
		emptyResponse := CredentialProviderResponse{
			APIVersion: apiVersion,
			Kind:       "CredentialProviderResponse",
			Auth:       make(map[string]AuthConfig),
		}
		json.NewEncoder(os.Stdout).Encode(emptyResponse)
		return
	}

	debug("Image matches registry, fetching credentials...")

	// Get fresh token from registry API
	token, expiresIn, err := fetchToken(config)
	if err != nil {
		fatal("Failed to fetch token: %v", err)
	}

	debug("Token fetched successfully (expires in %s)", expiresIn)

	// Parse cache duration
	cacheDuration, err := time.ParseDuration(config.CacheDuration)
	if err != nil {
		cacheDuration = 5 * time.Minute // Default 5 minutes
	}

	// Ensure cache duration is less than token expiration
	if cacheDuration > expiresIn {
		cacheDuration = expiresIn - 30*time.Second // Leave 30s buffer
	}

	// Build response
	response := CredentialProviderResponse{
		APIVersion: apiVersion,
		Kind:       "CredentialProviderResponse",
		CacheKeyType: "Image",
		CacheDuration: &Duration{Duration: cacheDuration},
		Auth: map[string]AuthConfig{
			registry: {
				Username: "token",
				Password: token,
			},
		},
	}

	// Return credentials to kubelet via stdout
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		fatal("Failed to encode response: %v", err)
	}

	debug("Credentials returned successfully")
}

// loadConfig loads configuration from environment variables or config file
func loadConfig() (*Config, error) {
	config := &Config{
		RegistryURL:   getEnv("REGISTRY_URL", "apps.afterdarksys.com:5005"),
		TokenURL:      getEnv("TOKEN_URL", "https://apps.afterdarksys.com:5005/api/v2/k8s/token"),
		TokenFile:     getEnv("TOKEN_FILE", "/var/lib/kubelet/registry-token"),
		CacheDuration: getEnv("CACHE_DURATION", "5m"),
	}

	// Validate config
	if config.RegistryURL == "" {
		return nil, fmt.Errorf("REGISTRY_URL is required")
	}

	if config.TokenURL == "" {
		return nil, fmt.Errorf("TOKEN_URL is required")
	}

	return config, nil
}

// fetchToken fetches a short-lived token from the registry API
func fetchToken(config *Config) (string, time.Duration, error) {
	// Read authentication token from file
	authToken, err := os.ReadFile(config.TokenFile)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read token file %s: %w", config.TokenFile, err)
	}

	// Create request to token API
	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		config.TokenURL,
		nil,
	)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", string(authToken)))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to fetch token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("token API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResponse struct {
		Token     string `json:"token"`
		ExpiresIn int64  `json:"expires_in"` // Seconds
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", 0, fmt.Errorf("failed to decode token response: %w", err)
	}

	expiresIn := time.Duration(tokenResponse.ExpiresIn) * time.Second

	return tokenResponse.Token, expiresIn, nil
}

// extractRegistry extracts the registry hostname from an image reference
func extractRegistry(image string) string {
	// Examples:
	// apps.afterdarksys.com:5005/myapp/frontend:latest → apps.afterdarksys.com:5005
	// gcr.io/project/image:tag → gcr.io
	// ubuntu:latest → docker.io (implicit)

	// Split on first slash
	parts := splitN(image, "/", 2)

	// If no slash, it's a Docker Hub image (docker.io)
	if len(parts) == 1 {
		return "docker.io"
	}

	// If first part contains dot or colon, it's a registry
	firstPart := parts[0]
	if contains(firstPart, ".") || contains(firstPart, ":") {
		return firstPart
	}

	// Otherwise, it's docker.io/username/repo
	return "docker.io"
}

// matchesRegistry checks if the image registry matches our registry
func matchesRegistry(imageRegistry, configRegistry string) bool {
	// Remove port if present for comparison
	imageHost := stripPort(imageRegistry)
	configHost := stripPort(configRegistry)

	return imageHost == configHost || imageRegistry == configRegistry
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func debug(format string, args ...interface{}) {
	if os.Getenv("DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[credential-provider] "+format+"\n", args...)
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[credential-provider] ERROR: "+format+"\n", args...)
	os.Exit(1)
}

func splitN(s, sep string, n int) []string {
	result := []string{}
	current := ""
	count := 0

	for _, char := range s {
		if string(char) == sep && count < n-1 {
			result = append(result, current)
			current = ""
			count++
		} else {
			current += string(char)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func stripPort(host string) string {
	// Remove port if present
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
		if host[i] == '/' {
			break
		}
	}
	return host
}
