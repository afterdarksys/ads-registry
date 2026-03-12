package db

import (
	"context"
	"errors"
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
	ListManifests(ctx context.Context) ([]ManifestRecord, error)

	// Blobs
	PutBlob(ctx context.Context, digest string, size int64, mediaType string) error
	BlobExists(ctx context.Context, digest string) (bool, error)
	GetBlobSize(ctx context.Context, digest string) (int64, error)

	// Scanning
	SaveScanReport(ctx context.Context, digest string, scanner string, data []byte) error
	GetScanReport(ctx context.Context, digest string, scanner string) ([]byte, error)

	// Users/Auth
	GetUserByToken(ctx context.Context, token string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	AuthenticateUser(ctx context.Context, username, password string) (*User, error)
	CreateUser(ctx context.Context, username, passwordHash string, scopes []string) error
	ListUsers(ctx context.Context) ([]User, error)

	// Groups and Quotas
	CreateGroup(ctx context.Context, name string) error
	AddUserToGroup(ctx context.Context, username, groupName string) error
	CheckQuota(ctx context.Context, namespace string) (*Quota, error)
	SetQuota(ctx context.Context, namespace string, limitBytes int64) error
	UpdateQuotaUsage(ctx context.Context, namespace string, sizeDelta int64) error
	ListGroups(ctx context.Context) ([]Group, error)
	ListQuotas(ctx context.Context) ([]Quota, error)

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
