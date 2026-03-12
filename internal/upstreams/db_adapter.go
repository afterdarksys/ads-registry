package upstreams

import (
	"context"
	"fmt"
	"time"
)

// DBStore is the database interface we expect from postgres/sqlite stores
type DBStore interface {
	CreateUpstream(ctx context.Context, name, upstreamType, endpoint, region, accessKeyID, secretAccessKey string, enabled, cacheEnabled, pullOnly bool, cacheTTL int) (int, error)
	GetUpstream(ctx context.Context, id int) (map[string]interface{}, error)
	GetUpstreamByName(ctx context.Context, name string) (map[string]interface{}, error)
	ListUpstreams(ctx context.Context) ([]map[string]interface{}, error)
	UpdateUpstreamToken(ctx context.Context, id int, token string, expiry time.Time) error
	DeleteUpstream(ctx context.Context, id int) error
}

// StoreAdapter adapts the generic DB interface to the upstreams.Store interface
type StoreAdapter struct {
	db DBStore
}

// NewStoreAdapter creates a new store adapter
func NewStoreAdapter(db DBStore) *StoreAdapter {
	return &StoreAdapter{db: db}
}

// CreateUpstream creates a new upstream registry
func (a *StoreAdapter) CreateUpstream(ctx context.Context, upstream *UpstreamRegistry) error {
	id, err := a.db.CreateUpstream(
		ctx,
		upstream.Name,
		string(upstream.Type),
		upstream.Endpoint,
		upstream.Region,
		upstream.AccessKeyID,
		upstream.SecretAccessKey,
		upstream.Enabled,
		upstream.CacheEnabled,
		upstream.PullOnly,
		upstream.CacheTTL,
	)
	if err != nil {
		return err
	}
	upstream.ID = id
	return nil
}

// GetUpstream retrieves an upstream by ID
func (a *StoreAdapter) GetUpstream(ctx context.Context, id int) (*UpstreamRegistry, error) {
	data, err := a.db.GetUpstream(ctx, id)
	if err != nil {
		return nil, err
	}
	return mapToUpstream(data)
}

// GetUpstreamByName retrieves an upstream by name
func (a *StoreAdapter) GetUpstreamByName(ctx context.Context, name string) (*UpstreamRegistry, error) {
	data, err := a.db.GetUpstreamByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return mapToUpstream(data)
}

// ListUpstreams returns all upstreams
func (a *StoreAdapter) ListUpstreams(ctx context.Context) ([]*UpstreamRegistry, error) {
	dataList, err := a.db.ListUpstreams(ctx)
	if err != nil {
		return nil, err
	}

	upstreams := make([]*UpstreamRegistry, len(dataList))
	for i, data := range dataList {
		upstream, err := mapToUpstream(data)
		if err != nil {
			return nil, err
		}
		upstreams[i] = upstream
	}
	return upstreams, nil
}

// UpdateUpstream updates an upstream
func (a *StoreAdapter) UpdateUpstream(ctx context.Context, upstream *UpstreamRegistry) error {
	// For now we just update the token using UpdateToken
	// If we need full updates later, we can add an UpdateUpstream method to DBStore
	return a.UpdateToken(ctx, upstream.ID, upstream.CurrentToken, upstream.TokenExpiry)
}

// DeleteUpstream deletes an upstream
func (a *StoreAdapter) DeleteUpstream(ctx context.Context, id int) error {
	return a.db.DeleteUpstream(ctx, id)
}

// UpdateToken updates the token for an upstream
func (a *StoreAdapter) UpdateToken(ctx context.Context, id int, token string, expiry time.Time) error {
	return a.db.UpdateUpstreamToken(ctx, id, token, expiry)
}

// mapToUpstream converts a map[string]interface{} to *UpstreamRegistry
func mapToUpstream(data map[string]interface{}) (*UpstreamRegistry, error) {
	upstream := &UpstreamRegistry{}

	if id, ok := data["id"].(int); ok {
		upstream.ID = id
	} else if id64, ok := data["id"].(int64); ok {
		upstream.ID = int(id64)
	}

	if name, ok := data["name"].(string); ok {
		upstream.Name = name
	}

	if upstreamType, ok := data["type"].(string); ok {
		upstream.Type = UpstreamType(upstreamType)
	}

	if endpoint, ok := data["endpoint"].(string); ok {
		upstream.Endpoint = endpoint
	}

	if region, ok := data["region"].(string); ok {
		upstream.Region = region
	}

	if accessKeyID, ok := data["access_key_id"].(string); ok {
		upstream.AccessKeyID = accessKeyID
	}

	if secretAccessKey, ok := data["secret_access_key"].(string); ok {
		upstream.SecretAccessKey = secretAccessKey
	}

	if currentToken, ok := data["current_token"].(string); ok {
		upstream.CurrentToken = currentToken
	}

	if tokenExpiry, ok := data["token_expiry"].(time.Time); ok {
		upstream.TokenExpiry = tokenExpiry
	}

	if lastRefresh, ok := data["last_refresh"].(time.Time); ok {
		upstream.LastRefresh = lastRefresh
	}

	if refreshFailCount, ok := data["refresh_fail_count"].(int); ok {
		upstream.RefreshFailCount = refreshFailCount
	} else if refreshFailCount64, ok := data["refresh_fail_count"].(int64); ok {
		upstream.RefreshFailCount = int(refreshFailCount64)
	}

	if enabled, ok := data["enabled"].(bool); ok {
		upstream.Enabled = enabled
	}

	if cacheEnabled, ok := data["cache_enabled"].(bool); ok {
		upstream.CacheEnabled = cacheEnabled
	}

	if cacheTTL, ok := data["cache_ttl"].(int); ok {
		upstream.CacheTTL = cacheTTL
	} else if cacheTTL64, ok := data["cache_ttl"].(int64); ok {
		upstream.CacheTTL = int(cacheTTL64)
	}

	if pullOnly, ok := data["pull_only"].(bool); ok {
		upstream.PullOnly = pullOnly
	}

	if createdAt, ok := data["created_at"].(time.Time); ok {
		upstream.CreatedAt = createdAt
	}

	if updatedAt, ok := data["updated_at"].(time.Time); ok {
		upstream.UpdatedAt = updatedAt
	}

	if upstream.ID == 0 || upstream.Name == "" {
		return nil, fmt.Errorf("invalid upstream data: missing required fields")
	}

	return upstream, nil
}
