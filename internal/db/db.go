package db

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("record not found")
)

type Store interface {
	// Namespace and Repository Management
	CreateNamespace(ctx context.Context, name string) error
	CreateRepository(ctx context.Context, namespace, name string) error
	ListRepositories(ctx context.Context, limit int, last string) ([]string, error)
	ListTags(ctx context.Context, repo string, limit int, last string) ([]string, error)

	// Manifests
	PutManifest(ctx context.Context, repo, reference string, mediaType string, digest string, payload []byte) error
	GetManifest(ctx context.Context, repo, reference string) (mediaType, digest string, payload []byte, err error)
	DeleteManifest(ctx context.Context, repo, reference string) error
	ListManifests(ctx context.Context) ([]ManifestRecord, error)

	// Blobs
	PutBlob(ctx context.Context, digest string, size int64, mediaType string) error
	BlobExists(ctx context.Context, digest string) (bool, error)
	GetBlobSize(ctx context.Context, digest string) (int64, error)

	// Scanning
	SaveScanReport(ctx context.Context, digest string, scanner string, data []byte) error
	GetScanReport(ctx context.Context, digest string, scanner string) ([]byte, error)
	ListScanReports(ctx context.Context) ([]ScanReport, error)

	// Users/Auth
	GetUserByToken(ctx context.Context, token string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	AuthenticateUser(ctx context.Context, username, password string) (*User, error)
	CreateUser(ctx context.Context, username, passwordHash string, scopes []string) error
	ListUsers(ctx context.Context) ([]User, error)
	DeleteUser(ctx context.Context, username string) error
	UpdateUser(ctx context.Context, username string, scopes []string) error
	UpdateUserPassword(ctx context.Context, username, passwordHash string) error

	// Groups and Quotas
	CreateGroup(ctx context.Context, name string) error
	AddUserToGroup(ctx context.Context, username, groupName string) error
	CheckQuota(ctx context.Context, namespace string) (*Quota, error)
	SetQuota(ctx context.Context, namespace string, limitBytes int64) error
	UpdateQuotaUsage(ctx context.Context, namespace string, sizeDelta int64) error
	ListGroups(ctx context.Context) ([]Group, error)

	// Access Tokens (for Docker CLI auth when using OAuth2)
	CreateAccessToken(ctx context.Context, userID int, name, tokenHash string, scopes []string, expiresAt *time.Time) (int, error)
	ListAccessTokens(ctx context.Context, userID int) ([]AccessToken, error)
	GetAccessTokenByHash(ctx context.Context, tokenHash string) (*AccessToken, error)
	DeleteAccessToken(ctx context.Context, tokenID int) error
	UpdateAccessTokenLastUsed(ctx context.Context, tokenID int) error
	ListQuotas(ctx context.Context) ([]Quota, error)

	// Upstream Registries
	GetUpstream(ctx context.Context, id int) (map[string]interface{}, error)
	GetUpstreamByName(ctx context.Context, name string) (map[string]interface{}, error)
	ListUpstreams(ctx context.Context) ([]map[string]interface{}, error)

	// OCI Artifacts (Referrers API, Helm charts, etc.)
	SetArtifactMetadata(ctx context.Context, metadata *ArtifactMetadata) error
	GetArtifactMetadata(ctx context.Context, digest string) (*ArtifactMetadata, error)
	ListReferrers(ctx context.Context, subjectDigest string, artifactType string) ([]ReferrerDescriptor, error)
	ListArtifactsByType(ctx context.Context, artifactType string, limit int) ([]ArtifactDescriptor, error)

	Close() error
}

type User struct {
	ID           int
	Username     string
	PasswordHash string
	Scopes       []string // Simplified RBAC for MVP
}

type Group struct {
	ID   int
	Name string
}

type AccessToken struct {
	ID         int
	UserID     int
	Name       string
	TokenHash  string
	Scopes     []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
}

type Quota struct {
	NamespaceID int
	Namespace   string
	LimitBytes  int64
	UsedBytes   int64
}

// Minimal stub for ScanReport in DB context if needed later
type ScanReport struct {
	Digest  string
	Scanner string
	Data    []byte
}

type ManifestRecord struct {
	Namespace string
	Repo      string
	Reference string
	Digest    string
}

// ArtifactMetadata represents OCI artifact metadata
type ArtifactMetadata struct {
	Digest         string
	ArtifactType   string
	SubjectDigest  string
	ChartName      string
	ChartVersion   string
	AppVersion     string
	MetadataJSON   string
}

// ReferrerDescriptor describes an artifact that refers to a subject
type ReferrerDescriptor struct {
	Digest       string
	MediaType    string
	ArtifactType string
	Size         int64
	Annotations  map[string]string
}

// ArtifactDescriptor describes an OCI artifact
type ArtifactDescriptor struct {
	Digest       string
	Namespace    string
	Repo         string
	ArtifactType string
	MediaType    string
	Size         int64
	CreatedAt    string
	// Helm-specific fields
	ChartName    string
	ChartVersion string
	AppVersion   string
}
