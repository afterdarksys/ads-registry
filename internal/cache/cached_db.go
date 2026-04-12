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

// DeleteManifest removes a manifest and invalidates cache
func (s *CachedStore) DeleteManifest(ctx context.Context, repo, reference string) error {
	if err := s.db.DeleteManifest(ctx, repo, reference); err != nil {
		return err
	}

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

func (s *CachedStore) ListScanReports(ctx context.Context) ([]db.ScanReport, error) {
	return s.db.ListScanReports(ctx)
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

func (s *CachedStore) DeleteBlob(ctx context.Context, digest string) error {
	return s.db.DeleteBlob(ctx, digest)
}

func (s *CachedStore) ListBlobs(ctx context.Context) ([]db.BlobRecord, error) {
	return s.db.ListBlobs(ctx)
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

func (s *CachedStore) DeleteUser(ctx context.Context, username string) error {
	return s.db.DeleteUser(ctx, username)
}

func (s *CachedStore) UpdateUser(ctx context.Context, username string, scopes []string) error {
	return s.db.UpdateUser(ctx, username, scopes)
}

func (s *CachedStore) UpdateUserPassword(ctx context.Context, username, passwordHash string) error {
	return s.db.UpdateUserPassword(ctx, username, passwordHash)
}

func (s *CachedStore) ListGroups(ctx context.Context) ([]db.Group, error) {
	return s.db.ListGroups(ctx)
}

func (s *CachedStore) ListQuotas(ctx context.Context) ([]db.Quota, error) {
	return s.db.ListQuotas(ctx)
}

func (s *CachedStore) GetUpstream(ctx context.Context, id int) (map[string]interface{}, error) {
	return s.db.GetUpstream(ctx, id)
}

func (s *CachedStore) GetUpstreamByName(ctx context.Context, name string) (map[string]interface{}, error) {
	return s.db.GetUpstreamByName(ctx, name)
}

func (s *CachedStore) ListUpstreams(ctx context.Context) ([]map[string]interface{}, error) {
	return s.db.ListUpstreams(ctx)
}

// OCI Artifacts - pass-through (no caching for now)
func (s *CachedStore) SetArtifactMetadata(ctx context.Context, metadata *db.ArtifactMetadata) error {
	return s.db.SetArtifactMetadata(ctx, metadata)
}

func (s *CachedStore) GetArtifactMetadata(ctx context.Context, digest string) (*db.ArtifactMetadata, error) {
	return s.db.GetArtifactMetadata(ctx, digest)
}

func (s *CachedStore) ListReferrers(ctx context.Context, subjectDigest string, artifactType string) ([]db.ReferrerDescriptor, error) {
	return s.db.ListReferrers(ctx, subjectDigest, artifactType)
}

func (s *CachedStore) ListArtifactsByType(ctx context.Context, artifactType string, limit int) ([]db.ArtifactDescriptor, error) {
	return s.db.ListArtifactsByType(ctx, artifactType, limit)
}

// Access Tokens - passthrough (no caching for auth tokens)
func (s *CachedStore) CreateAccessToken(ctx context.Context, userID int, name, tokenHash string, scopes []string, expiresAt *time.Time) (int, error) {
	return s.db.CreateAccessToken(ctx, userID, name, tokenHash, scopes, expiresAt)
}

func (s *CachedStore) ListAccessTokens(ctx context.Context, userID int) ([]db.AccessToken, error) {
	return s.db.ListAccessTokens(ctx, userID)
}

func (s *CachedStore) GetAccessTokenByHash(ctx context.Context, tokenHash string) (*db.AccessToken, error) {
	return s.db.GetAccessTokenByHash(ctx, tokenHash)
}

func (s *CachedStore) DeleteAccessToken(ctx context.Context, tokenID int) error {
	return s.db.DeleteAccessToken(ctx, tokenID)
}

func (s *CachedStore) UpdateAccessTokenLastUsed(ctx context.Context, tokenID int) error {
	return s.db.UpdateAccessTokenLastUsed(ctx, tokenID)
}

func (s *CachedStore) ListPolicies(ctx context.Context) ([]db.PolicyRecord, error) {
	return s.db.ListPolicies(ctx)
}

func (s *CachedStore) AddPolicy(ctx context.Context, expression string) error {
	return s.db.AddPolicy(ctx, expression)
}

func (s *CachedStore) DeletePolicy(ctx context.Context, id int) error {
	return s.db.DeletePolicy(ctx, id)
}

// Multi-format Artifacts - passthrough
func (s *CachedStore) CreateArtifact(ctx context.Context, artifact *db.UniversalArtifact) (int64, error) {
	return s.db.CreateArtifact(ctx, artifact)
}

func (s *CachedStore) GetArtifact(ctx context.Context, format, namespace, packageName, version string) (*db.UniversalArtifact, error) {
	return s.db.GetArtifact(ctx, format, namespace, packageName, version)
}

func (s *CachedStore) ListArtifacts(ctx context.Context, format, namespace, packageName string) ([]*db.UniversalArtifact, error) {
	return s.db.ListArtifacts(ctx, format, namespace, packageName)
}

func (s *CachedStore) SearchArtifacts(ctx context.Context, format, namespace string, searchQuery json.RawMessage) ([]*db.UniversalArtifact, error) {
	return s.db.SearchArtifacts(ctx, format, namespace, searchQuery)
}

func (s *CachedStore) StoreArtifactMetadata(ctx context.Context, artifactID int64, data json.RawMessage) error {
	return s.db.StoreArtifactMetadata(ctx, artifactID, data)
}

func (s *CachedStore) AttachBlob(ctx context.Context, artifactID int64, blobDigest, fileName string) error {
	return s.db.AttachBlob(ctx, artifactID, blobDigest, fileName)
}

func (s *CachedStore) DeleteArtifact(ctx context.Context, format, namespace, packageName, version string) error {
	return s.db.DeleteArtifact(ctx, format, namespace, packageName, version)
}

func (s *CachedStore) DeleteAllArtifactVersions(ctx context.Context, format, namespace, packageName string) error {
	return s.db.DeleteAllArtifactVersions(ctx, format, namespace, packageName)
}

func (s *CachedStore) GetPackageNames(ctx context.Context, format, namespace string) ([]string, error) {
	return s.db.GetPackageNames(ctx, format, namespace)
}

func (s *CachedStore) GetArtifactStatistics(ctx context.Context, format, namespace string) (*db.ArtifactStatistics, error) {
	return s.db.GetArtifactStatistics(ctx, format, namespace)
}

func (s *CachedStore) SetTagImmutable(ctx context.Context, repo, reference string, immutable bool) error {
	return s.db.SetTagImmutable(ctx, repo, reference, immutable)
}

func (s *CachedStore) IsTagImmutable(ctx context.Context, repo, reference string) (bool, error) {
	return s.db.IsTagImmutable(ctx, repo, reference)
}

func (s *CachedStore) WithTx(ctx context.Context, fn func(context.Context) error) error {
	return s.db.WithTx(ctx, fn)
}

func (s *CachedStore) Close() error {
	return s.db.Close()
}
