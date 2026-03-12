package upstreams

import (
	"context"
	"time"
)

// UpstreamType represents the cloud provider type
type UpstreamType string

const (
	UpstreamTypeAWS    UpstreamType = "aws"
	UpstreamTypeOracle UpstreamType = "oracle"
	UpstreamTypeGCP    UpstreamType = "gcp"
	UpstreamTypeAzure  UpstreamType = "azure"
	UpstreamTypeDocker UpstreamType = "dockerhub"
)

// UpstreamRegistry represents a configured upstream container registry
type UpstreamRegistry struct {
	ID          int
	Name        string       // User-friendly name (e.g., "production-ecr")
	Type        UpstreamType // Cloud provider type
	Endpoint    string       // Registry endpoint URL
	Region      string       // Cloud region (AWS, Oracle, GCP)

	// Credentials (encrypted in DB, or Vault reference)
	AccessKeyID     string // AWS access key, Oracle user OCID, etc.
	SecretAccessKey string // AWS secret key, Oracle private key, etc.

	// Token state
	CurrentToken     string    // Current valid token
	TokenExpiry      time.Time // When current token expires
	LastRefresh      time.Time // Last successful token refresh
	RefreshFailCount int       // Consecutive refresh failures

	// Configuration
	Enabled       bool      // Whether this upstream is active
	CacheEnabled  bool      // Cache pulled images locally
	CacheTTL      int       // Cache duration in seconds
	PullOnly      bool      // If true, only allow pulls (no pushes)

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Provider defines the interface for upstream registry providers
type Provider interface {
	// Name returns the provider type
	Name() UpstreamType

	// RefreshToken fetches a new authentication token from the cloud provider
	RefreshToken(ctx context.Context, registry *UpstreamRegistry) (token string, expiry time.Time, err error)

	// ValidateCredentials checks if the stored credentials are valid
	ValidateCredentials(ctx context.Context, registry *UpstreamRegistry) error

	// GetRegistryEndpoint returns the full registry URL for the given repository
	GetRegistryEndpoint(registry *UpstreamRegistry, repository string) string

	// NeedsRefresh returns true if the token should be refreshed
	NeedsRefresh(registry *UpstreamRegistry) bool
}

// CredentialConfig holds cloud provider credentials for adding an upstream
type CredentialConfig struct {
	// AWS ECR
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSRegion          string
	AWSAccountID       string

	// Oracle OCI
	OCITenancyID   string
	OCIUserID      string
	OCIFingerprint string
	OCIPrivateKey  string // PEM-encoded RSA private key
	OCIRegion      string

	// GCP Artifact Registry
	GCPProjectID      string
	GCPServiceAccount string // JSON key file content
	GCPRegion         string

	// Azure ACR
	AzureClientID     string
	AzureClientSecret string
	AzureTenantID     string
	AzureRegistryName string

	// Docker Hub
	DockerHubUsername string
	DockerHubPassword string
}

// TokenRefreshJob represents a River job for refreshing upstream tokens
type TokenRefreshJob struct {
	UpstreamID int
}

// ProxyRequest represents a proxied pull/push request
type ProxyRequest struct {
	UpstreamName string
	Repository   string
	Reference    string
	Method       string
}
