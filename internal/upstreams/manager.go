package upstreams

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Manager handles upstream registry management and token refresh
type Manager struct {
	providers map[UpstreamType]Provider
	store     Store
	mu        sync.RWMutex
}

// Store defines the database interface for upstream registries
type Store interface {
	// CreateUpstream adds a new upstream registry
	CreateUpstream(ctx context.Context, upstream *UpstreamRegistry) error

	// GetUpstream retrieves an upstream by ID
	GetUpstream(ctx context.Context, id int) (*UpstreamRegistry, error)

	// GetUpstreamByName retrieves an upstream by name
	GetUpstreamByName(ctx context.Context, name string) (*UpstreamRegistry, error)

	// ListUpstreams returns all configured upstreams
	ListUpstreams(ctx context.Context) ([]*UpstreamRegistry, error)

	// UpdateUpstream updates an existing upstream
	UpdateUpstream(ctx context.Context, upstream *UpstreamRegistry) error

	// DeleteUpstream removes an upstream
	DeleteUpstream(ctx context.Context, id int) error

	// UpdateToken updates the current token and expiry for an upstream
	UpdateToken(ctx context.Context, id int, token string, expiry time.Time) error
}

// NewManager creates a new upstream manager
func NewManager(store Store) *Manager {
	m := &Manager{
		providers: make(map[UpstreamType]Provider),
		store:     store,
	}

	// Register built-in providers
	m.RegisterProvider(NewAWSProvider())
	m.RegisterProvider(NewOracleProvider())
	m.RegisterProvider(NewDockerHubProvider())
	m.RegisterProvider(NewGCPProvider())

	return m
}

// RegisterProvider registers a cloud provider
func (m *Manager) RegisterProvider(provider Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[provider.Name()] = provider
}

// AddUpstream adds a new upstream registry with credentials
func (m *Manager) AddUpstream(ctx context.Context, name string, upstreamType UpstreamType, config *CredentialConfig) (*UpstreamRegistry, error) {
	m.mu.RLock()
	provider, exists := m.providers[upstreamType]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unsupported upstream type: %s", upstreamType)
	}

	// Create upstream record
	upstream := &UpstreamRegistry{
		Name:        name,
		Type:        upstreamType,
		Enabled:     true,
		CacheEnabled: true,
		CacheTTL:    3600, // 1 hour default
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Set provider-specific fields
	switch upstreamType {
	case UpstreamTypeAWS:
		upstream.Region = config.AWSRegion
		upstream.AccessKeyID = config.AWSAccessKeyID
		upstream.SecretAccessKey = config.AWSSecretAccessKey
		upstream.Endpoint = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", config.AWSAccountID, config.AWSRegion)

	case UpstreamTypeOracle:
		upstream.Region = config.OCIRegion
		upstream.AccessKeyID = config.OCIUserID
		upstream.SecretAccessKey = config.OCIPrivateKey
		upstream.Endpoint = fmt.Sprintf("%s.ocir.io", config.OCIRegion)

	case UpstreamTypeDocker:
		upstream.Region = "us" // Docker Hub doesn't have regions
		upstream.AccessKeyID = config.DockerHubUsername
		upstream.SecretAccessKey = config.DockerHubPassword
		upstream.Endpoint = "registry-1.docker.io"

	case UpstreamTypeGCP:
		upstream.Region = config.GCPRegion
		upstream.AccessKeyID = config.GCPProjectID
		upstream.SecretAccessKey = config.GCPServiceAccount
		// Artifact Registry format
		upstream.Endpoint = fmt.Sprintf("%s-docker.pkg.dev/%s", config.GCPRegion, config.GCPProjectID)
	}

	// Validate credentials
	if err := provider.ValidateCredentials(ctx, upstream); err != nil {
		return nil, fmt.Errorf("credential validation failed: %w", err)
	}

	// Get initial token
	token, expiry, err := provider.RefreshToken(ctx, upstream)
	if err != nil {
		return nil, fmt.Errorf("failed to get initial token: %w", err)
	}

	upstream.CurrentToken = token
	upstream.TokenExpiry = expiry
	upstream.LastRefresh = time.Now()

	// Save to database
	if err := m.store.CreateUpstream(ctx, upstream); err != nil {
		return nil, fmt.Errorf("failed to save upstream: %w", err)
	}

	return upstream, nil
}

// RefreshUpstreamToken refreshes the token for an upstream registry
func (m *Manager) RefreshUpstreamToken(ctx context.Context, upstreamID int) error {
	upstream, err := m.store.GetUpstream(ctx, upstreamID)
	if err != nil {
		return fmt.Errorf("failed to get upstream: %w", err)
	}

	if !upstream.Enabled {
		return fmt.Errorf("upstream %s is disabled", upstream.Name)
	}

	m.mu.RLock()
	provider, exists := m.providers[upstream.Type]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("provider not found for type: %s", upstream.Type)
	}

	// Refresh the token
	token, expiry, err := provider.RefreshToken(ctx, upstream)
	if err != nil {
		upstream.RefreshFailCount++
		m.store.UpdateUpstream(ctx, upstream)
		return fmt.Errorf("token refresh failed: %w", err)
	}

	// Update token in database
	if err := m.store.UpdateToken(ctx, upstreamID, token, expiry); err != nil {
		return fmt.Errorf("failed to update token: %w", err)
	}

	upstream.RefreshFailCount = 0
	upstream.LastRefresh = time.Now()

	return nil
}

// GetUpstreamToken returns a valid token for an upstream, refreshing if needed
func (m *Manager) GetUpstreamToken(ctx context.Context, upstreamName string) (string, error) {
	upstream, err := m.store.GetUpstreamByName(ctx, upstreamName)
	if err != nil {
		return "", fmt.Errorf("upstream not found: %w", err)
	}

	if !upstream.Enabled {
		return "", fmt.Errorf("upstream %s is disabled", upstreamName)
	}

	m.mu.RLock()
	provider, exists := m.providers[upstream.Type]
	m.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("provider not found for type: %s", upstream.Type)
	}

	// Check if token needs refresh
	if provider.NeedsRefresh(upstream) {
		if err := m.RefreshUpstreamToken(ctx, upstream.ID); err != nil {
			return "", err
		}
		// Reload upstream to get fresh token
		upstream, err = m.store.GetUpstream(ctx, upstream.ID)
		if err != nil {
			return "", err
		}
	}

	return upstream.CurrentToken, nil
}

// ListUpstreams returns all configured upstreams
func (m *Manager) ListUpstreams(ctx context.Context) ([]*UpstreamRegistry, error) {
	return m.store.ListUpstreams(ctx)
}

// DeleteUpstream removes an upstream registry
func (m *Manager) DeleteUpstream(ctx context.Context, name string) error {
	upstream, err := m.store.GetUpstreamByName(ctx, name)
	if err != nil {
		return err
	}
	return m.store.DeleteUpstream(ctx, upstream.ID)
}
