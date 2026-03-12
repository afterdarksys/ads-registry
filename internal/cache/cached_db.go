package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ryan/ads-registry/internal/db"
)

// CachedStore wraps a db.Store with Redis caching
type CachedStore struct {
	db    db.Store
	cache *RedisCache
	ttl   TTLConfig
}

// TTLConfig holds TTL values for different cache types
type TTLConfig struct {
	Manifest   time.Duration
	Signature  time.Duration
	ScanReport time.Duration
	Policy     time.Duration
}

// NewCachedStore creates a cached database store
func NewCachedStore(dbStore db.Store, cache *RedisCache, ttl TTLConfig) *CachedStore {
	return &CachedStore{
		db:    dbStore,
		cache: cache,
		ttl:   ttl,
	}
}

// GetManifest retrieves a manifest with caching
func (s *CachedStore) GetManifest(ctx context.Context, repo, reference string) (mediaType, digest string, payload []byte, err error) {
	// Try cache first
	if s.cache != nil {
		// Extract namespace and repo name from repo path
		cacheKey := BuildManifestKey("", repo, reference)
		cachedData, err := s.cache.Get(ctx, cacheKey)
		if err == nil && cachedData != nil {
			// Unmarshal cached data
			var cached struct {
				MediaType string `json:"media_type"`
				Digest    string `json:"digest"`
				Payload   []byte `json:"payload"`
			}
			if err := json.Unmarshal(cachedData, &cached); err == nil {
				return cached.MediaType, cached.Digest, cached.Payload, nil
			}
		}
	}

	// Cache miss, fetch from database
	mediaType, digest, payload, err = s.db.GetManifest(ctx, repo, reference)
	if err != nil {
		return "", "", nil, err
	}

	// Store in cache
	if s.cache != nil {
		cacheKey := BuildManifestKey("", repo, reference)
		cacheData := struct {
			MediaType string `json:"media_type"`
			Digest    string `json:"digest"`
			Payload   []byte `json:"payload"`
		}{
			MediaType: mediaType,
			Digest:    digest,
			Payload:   payload,
		}
		if data, err := json.Marshal(cacheData); err == nil {
			s.cache.Set(ctx, cacheKey, data, s.ttl.Manifest)
		}
	}

	return mediaType, digest, payload, nil
}

// PutManifest stores a manifest and invalidates cache
func (s *CachedStore) PutManifest(ctx context.Context, repo, reference string, mediaType string, digest string, payload []byte) error {
	// Store in database
	if err := s.db.PutManifest(ctx, repo, reference, mediaType, digest, payload); err != nil {
		return err
	}

	// Invalidate cache
	if s.cache != nil {
		cacheKey := BuildManifestKey("", repo, reference)
		s.cache.Delete(ctx, cacheKey)
	}

	return nil
}

// GetScanReport retrieves a scan report with caching
func (s *CachedStore) GetScanReport(ctx context.Context, digest string, scanner string) ([]byte, error) {
	// Try cache first
	if s.cache != nil {
		cacheKey := BuildScanReportKey(digest, scanner)
		cachedData, err := s.cache.Get(ctx, cacheKey)
		if err == nil && cachedData != nil {
			return cachedData, nil
		}
	}

	// Cache miss, fetch from database
	data, err := s.db.GetScanReport(ctx, digest, scanner)
	if err != nil {
		return nil, err
	}

	// Store in cache
	if s.cache != nil {
		cacheKey := BuildScanReportKey(digest, scanner)
		s.cache.Set(ctx, cacheKey, data, s.ttl.ScanReport)
	}

	return data, nil
}

// SaveScanReport stores a scan report and updates cache
func (s *CachedStore) SaveScanReport(ctx context.Context, digest string, scanner string, data []byte) error {
	// Store in database
	if err := s.db.SaveScanReport(ctx, digest, scanner, data); err != nil {
		return err
	}

	// Update cache
	if s.cache != nil {
		cacheKey := BuildScanReportKey(digest, scanner)
		s.cache.Set(ctx, cacheKey, data, s.ttl.ScanReport)
	}

	return nil
}

// Pass-through methods (delegate to underlying db.Store)

func (s *CachedStore) CreateNamespace(ctx context.Context, name string) error {
	return s.db.CreateNamespace(ctx, name)
}

func (s *CachedStore) CreateRepository(ctx context.Context, namespace, name string) error {
	return s.db.CreateRepository(ctx, namespace, name)
}

func (s *CachedStore) ListRepositories(ctx context.Context, limit int, last string) ([]string, error) {
	return s.db.ListRepositories(ctx, limit, last)
}

func (s *CachedStore) ListTags(ctx context.Context, repo string, limit int, last string) ([]string, error) {
	return s.db.ListTags(ctx, repo, limit, last)
}

func (s *CachedStore) ListManifests(ctx context.Context) ([]db.ManifestRecord, error) {
	return s.db.ListManifests(ctx)
}

func (s *CachedStore) PutBlob(ctx context.Context, digest string, size int64, mediaType string) error {
	return s.db.PutBlob(ctx, digest, size, mediaType)
}

func (s *CachedStore) BlobExists(ctx context.Context, digest string) (bool, error) {
	return s.db.BlobExists(ctx, digest)
}

func (s *CachedStore) GetBlobSize(ctx context.Context, digest string) (int64, error) {
	return s.db.GetBlobSize(ctx, digest)
}

func (s *CachedStore) GetUserByToken(ctx context.Context, token string) (*db.User, error) {
	return s.db.GetUserByToken(ctx, token)
}

func (s *CachedStore) GetUserByUsername(ctx context.Context, username string) (*db.User, error) {
	return s.db.GetUserByUsername(ctx, username)
}

func (s *CachedStore) AuthenticateUser(ctx context.Context, username, password string) (*db.User, error) {
	return s.db.AuthenticateUser(ctx, username, password)
}

func (s *CachedStore) CreateUser(ctx context.Context, username, passwordHash string, scopes []string) error {
	return s.db.CreateUser(ctx, username, passwordHash, scopes)
}

func (s *CachedStore) CreateGroup(ctx context.Context, name string) error {
	return s.db.CreateGroup(ctx, name)
}

func (s *CachedStore) AddUserToGroup(ctx context.Context, username, groupName string) error {
	return s.db.AddUserToGroup(ctx, username, groupName)
}

func (s *CachedStore) CheckQuota(ctx context.Context, namespace string) (*db.Quota, error) {
	return s.db.CheckQuota(ctx, namespace)
}

func (s *CachedStore) SetQuota(ctx context.Context, namespace string, limitBytes int64) error {
	return s.db.SetQuota(ctx, namespace, limitBytes)
}

func (s *CachedStore) UpdateQuotaUsage(ctx context.Context, namespace string, sizeDelta int64) error {
	return s.db.UpdateQuotaUsage(ctx, namespace, sizeDelta)
}

func (s *CachedStore) ListUsers(ctx context.Context) ([]db.User, error) {
	return s.db.ListUsers(ctx)
}

func (s *CachedStore) ListGroups(ctx context.Context) ([]db.Group, error) {
	return s.db.ListGroups(ctx)
}

func (s *CachedStore) ListQuotas(ctx context.Context) ([]db.Quota, error) {
	return s.db.ListQuotas(ctx)
}

func (s *CachedStore) Close() error {
	return s.db.Close()
}
