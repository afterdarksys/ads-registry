package db

import (
	"context"
	"encoding/json"
	"time"
)

// UniversalArtifact represents a generic package manager artifact (NPM, PyPI, Helm, etc.)
type UniversalArtifact struct {
	ID          int64
	Format      string
	Namespace   string
	PackageName string
	Version     string
	CreatedAt   time.Time
	
	// Relationships
	Blobs    []ArtifactBlob
	Metadata json.RawMessage
}

// ArtifactBlob links an artifact to a specific file stored via OCI/storage drivers
type ArtifactBlob struct {
	ArtifactID int64
	BlobDigest string
	FileName   string
	CreatedAt  time.Time
}

// ArtifactStore extends the base Store interface to support Universal Artifact registries
type ArtifactStore interface {
	// CreateArtifact registers a new package version into the generic repository
	CreateArtifact(ctx context.Context, artifact *UniversalArtifact) (int64, error)

	// GetArtifact retrieves a specific version of a package
	GetArtifact(ctx context.Context, format, namespace, packageName, version string) (*UniversalArtifact, error)

	// ListArtifacts returns all versions for a package
	ListArtifacts(ctx context.Context, format, namespace, packageName string) ([]*UniversalArtifact, error)

	// SearchArtifacts allows querying metadata using JSONB operators
	SearchArtifacts(ctx context.Context, format, namespace string, searchQuery json.RawMessage) ([]*UniversalArtifact, error)

	// StoreArtifactMetadata saves format-specific JSON metadata
	StoreArtifactMetadata(ctx context.Context, artifactID int64, data json.RawMessage) error

	// AttachBlob links a stored blob to the artifact
	AttachBlob(ctx context.Context, artifactID int64, blobDigest, fileName string) error

	// DeleteArtifact removes a specific version of a package
	DeleteArtifact(ctx context.Context, format, namespace, packageName, version string) error

	// DeleteAllArtifactVersions removes all versions of a package
	DeleteAllArtifactVersions(ctx context.Context, format, namespace, packageName string) error

	// GetPackageNames returns unique package names for a format and namespace
	GetPackageNames(ctx context.Context, format, namespace string) ([]string, error)

	// GetArtifactStatistics returns usage statistics for artifacts
	GetArtifactStatistics(ctx context.Context, format, namespace string) (*ArtifactStatistics, error)
}

// ArtifactStatistics provides usage information about artifacts
type ArtifactStatistics struct {
	TotalPackages  int64
	TotalVersions  int64
	TotalSize      int64
	FormatBreakdown map[string]int64
}
