package db

import (
	"context"
	"encoding/json"
	"time"
)

// MockStore is a minimal mock implementation for testing
type MockStore struct{}

func (m *MockStore) GetUser(ctx context.Context, username string) (*User, error) {
	return nil, ErrNotFound
}

func (m *MockStore) CreateUser(ctx context.Context, username, passwordHash string, scopes []string) error {
	return nil
}

func (m *MockStore) ListRepositories(ctx context.Context, limit int, last string) ([]string, error) {
	return []string{}, nil
}

func (m *MockStore) ListTags(ctx context.Context, repo string, limit int, last string) ([]string, error) {
	return []string{}, nil
}

func (m *MockStore) GetManifest(ctx context.Context, repo, ref string) (mediaType, digest string, payload []byte, err error) {
	return "", "", nil, ErrNotFound
}

func (m *MockStore) DeleteManifest(ctx context.Context, repo, ref string) error {
	return nil
}

func (m *MockStore) PutManifest(ctx context.Context, repo, ref, mediaType, digest string, payload []byte) error {
	return nil
}

func (m *MockStore) GetBlobSize(ctx context.Context, digest string) (int64, error) {
	return 0, ErrNotFound
}

func (m *MockStore) BlobExists(ctx context.Context, digest string) (bool, error) {
	return false, nil
}

func (m *MockStore) PutBlob(ctx context.Context, digest string, size int64, mediaType string) error {
	return nil
}

func (m *MockStore) CheckQuota(ctx context.Context, namespace string) (*Quota, error) {
	return nil, nil
}

func (m *MockStore) UpdateQuotaUsage(ctx context.Context, namespace string, deltaBytes int64) error {
	return nil
}

func (m *MockStore) SetArtifactMetadata(ctx context.Context, metadata *ArtifactMetadata) error {
	return nil
}

func (m *MockStore) CreateNamespace(ctx context.Context, name string) error {
	return nil
}

func (m *MockStore) CreateRepository(ctx context.Context, namespace, name string) error {
	return nil
}

func (m *MockStore) ListManifests(ctx context.Context) ([]ManifestRecord, error) {
	return []ManifestRecord{}, nil
}

func (m *MockStore) SaveScanReport(ctx context.Context, digest string, scanner string, data []byte) error {
	return nil
}

func (m *MockStore) GetScanReport(ctx context.Context, digest string, scanner string) ([]byte, error) {
	return nil, ErrNotFound
}

func (m *MockStore) ListScanReports(ctx context.Context) ([]ScanReport, error) {
	return []ScanReport{}, nil
}

func (m *MockStore) GetUserByToken(ctx context.Context, token string) (*User, error) {
	return nil, ErrNotFound
}

func (m *MockStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return nil, ErrNotFound
}

func (m *MockStore) AuthenticateUser(ctx context.Context, username, password string) (*User, error) {
	return nil, ErrNotFound
}

func (m *MockStore) ListUsers(ctx context.Context) ([]User, error) {
	return []User{}, nil
}

func (m *MockStore) DeleteUser(ctx context.Context, username string) error {
	return nil
}

func (m *MockStore) UpdateUser(ctx context.Context, username string, scopes []string) error {
	return nil
}

func (m *MockStore) UpdateUserPassword(ctx context.Context, username, passwordHash string) error {
	return nil
}

func (m *MockStore) CreateGroup(ctx context.Context, name string) error {
	return nil
}

func (m *MockStore) AddUserToGroup(ctx context.Context, username, groupName string) error {
	return nil
}

func (m *MockStore) SetQuota(ctx context.Context, namespace string, limitBytes int64) error {
	return nil
}

func (m *MockStore) ListGroups(ctx context.Context) ([]Group, error) {
	return []Group{}, nil
}

func (m *MockStore) ListQuotas(ctx context.Context) ([]Quota, error) {
	return []Quota{}, nil
}

func (m *MockStore) GetUpstream(ctx context.Context, id int) (map[string]interface{}, error) {
	return nil, ErrNotFound
}

func (m *MockStore) GetUpstreamByName(ctx context.Context, name string) (map[string]interface{}, error) {
	return nil, ErrNotFound
}

func (m *MockStore) ListUpstreams(ctx context.Context) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

func (m *MockStore) GetArtifactMetadata(ctx context.Context, digest string) (*ArtifactMetadata, error) {
	return nil, ErrNotFound
}

func (m *MockStore) ListReferrers(ctx context.Context, subjectDigest string, artifactType string) ([]ReferrerDescriptor, error) {
	return []ReferrerDescriptor{}, nil
}

func (m *MockStore) ListArtifactsByType(ctx context.Context, artifactType string, limit int) ([]ArtifactDescriptor, error) {
	return []ArtifactDescriptor{}, nil
}

func (m *MockStore) CreateAccessToken(ctx context.Context, userID int, name, tokenHash string, scopes []string, expiresAt *time.Time) (int, error) {
	return 0, nil
}

func (m *MockStore) ListAccessTokens(ctx context.Context, userID int) ([]AccessToken, error) {
	return []AccessToken{}, nil
}

func (m *MockStore) GetAccessTokenByHash(ctx context.Context, tokenHash string) (*AccessToken, error) {
	return nil, ErrNotFound
}

func (m *MockStore) DeleteAccessToken(ctx context.Context, tokenID int) error {
	return nil
}

func (m *MockStore) UpdateAccessTokenLastUsed(ctx context.Context, tokenID int) error {
	return nil
}

func (m *MockStore) ListPolicies(ctx context.Context) ([]PolicyRecord, error) {
	return []PolicyRecord{}, nil
}

func (m *MockStore) AddPolicy(ctx context.Context, expression string) error {
	return nil
}

func (m *MockStore) DeletePolicy(ctx context.Context, id int) error {
	return nil
}

func (m *MockStore) CreateArtifact(ctx context.Context, artifact *UniversalArtifact) (int64, error) {
	return 0, nil
}

func (m *MockStore) GetArtifact(ctx context.Context, format, namespace, packageName, version string) (*UniversalArtifact, error) {
	return nil, ErrNotFound
}

func (m *MockStore) ListArtifacts(ctx context.Context, format, namespace, packageName string) ([]*UniversalArtifact, error) {
	return []*UniversalArtifact{}, nil
}

func (m *MockStore) SearchArtifacts(ctx context.Context, format, namespace string, searchQuery json.RawMessage) ([]*UniversalArtifact, error) {
	return []*UniversalArtifact{}, nil
}

func (m *MockStore) StoreArtifactMetadata(ctx context.Context, artifactID int64, data json.RawMessage) error {
	return nil
}

func (m *MockStore) AttachBlob(ctx context.Context, artifactID int64, blobDigest, fileName string) error {
	return nil
}

func (m *MockStore) Close() error {
	return nil
}

