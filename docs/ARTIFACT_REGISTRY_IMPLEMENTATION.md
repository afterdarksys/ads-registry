# Artifact Registry Implementation Summary

## Overview

This document provides a comprehensive overview of the universal artifact registry implementation for the ADS Registry project. The system now supports 8 different artifact formats with complete CRUD operations, security scanning, and a dedicated management CLI tool.

## Architecture

### Database Layer

#### Schema (`migrations/017_artifact_metadata.sql`)

Three core tables support all artifact formats:

1. **universal_artifacts** - Main artifact registry
   - `id`: Primary key
   - `format`: Package manager type (npm, pypi, helm, etc.)
   - `namespace`: Organizational namespace (default: "default")
   - `package_name`: Package identifier
   - `version`: Semantic version
   - `created_at`: Timestamp

2. **universal_artifact_blobs** - File storage references
   - Links artifacts to stored blobs (tarballs, wheels, debs, etc.)
   - Tracks checksums for integrity verification
   - Supports multiple files per artifact version

3. **universal_artifact_metadata** - Format-specific JSON metadata
   - Stores package.json, Chart.yaml, setup.py metadata
   - Uses JSONB (PostgreSQL) for efficient querying
   - Flexible schema per format

#### Database Implementations

**PostgreSQL** (`internal/db/postgres/artifacts.go`)
- Full JSONB support for advanced metadata queries
- Efficient GIN indexes on metadata fields
- Production-ready for high-volume operations

**SQLite** (`internal/db/sqlite/sqlite.go`)
- Complete artifact support (no longer stubbed)
- JSON text storage with standard indexes
- Suitable for development and small deployments

**Common Interface** (`internal/db/metadata.go`)
```go
type ArtifactStore interface {
    CreateArtifact(ctx, artifact) (id, error)
    GetArtifact(ctx, format, namespace, package, version) (*Artifact, error)
    ListArtifacts(ctx, format, namespace, package) ([]*Artifact, error)
    SearchArtifacts(ctx, format, namespace, query) ([]*Artifact, error)
    DeleteArtifact(ctx, format, namespace, package, version) error
    DeleteAllArtifactVersions(ctx, format, namespace, package) error
    GetPackageNames(ctx, format, namespace) ([]string, error)
    GetArtifactStatistics(ctx, format, namespace) (*Stats, error)
    StoreArtifactMetadata(ctx, artifactID, data) error
    AttachBlob(ctx, artifactID, digest, fileName) error
}
```

### Format Handlers

Each format has a dedicated handler in `internal/api/formats/`:

#### 1. npm (`npm/npm.go`)
- **Upload**: PUT with full package.json and base64-encoded tarballs
- **Download**: GET /{package}/-/{tarball}
- **Metadata**: npm-compatible package.json with dist.tarball URLs
- **Scopes**: Support for @scope/package naming
- **Features**:
  - Automatic dist-tags (latest)
  - Version resolution
  - npm CLI compatibility

#### 2. PyPI (`pypi/pypi.go`)
- **Upload**: POST multipart/form-data (twine compatible)
- **Download**: GET /packages/{version}/{filename}
- **Index**: PEP 503 simple repository API
- **Features**:
  - Wheel and sdist support
  - SHA256 checksums
  - HTML index generation
  - pip-compatible URLs

#### 3. Helm (`helm/helm.go`)
- **Upload**: POST with .tgz chart
- **Download**: GET /charts/{filename}
- **Index**: YAML index.yaml (Helm repository spec)
- **Features**:
  - Chart.yaml extraction
  - Version management
  - Repository index generation
  - helm CLI compatibility

#### 4. Go Modules (`golang/golang.go`)
- **Upload**: POST multipart with .zip + .mod
- **Download**: GOPROXY protocol endpoints
- **Features**:
  - /@v/list - version listing
  - /@v/{version}.info - version metadata
  - /@v/{version}.mod - go.mod file
  - /@v/{version}.zip - module source
  - Full GOPROXY compatibility

#### 5. APT (`apt/apt.go`)
- **Upload**: POST with .deb package
- **Download**: GET /pool/main/{prefix}/{package}/{filename}
- **Repository**: Debian repository structure
- **Features**:
  - control file extraction
  - Release file generation with MD5/SHA256
  - Packages manifest (plain and gzipped)
  - apt-get compatibility

#### 6. Composer (`composer/composer.go`)
- **Upload**: POST multipart with .zip
- **Download**: GET /dists/{vendor}/{package}/{filename}
- **Metadata**: packages.json and p2/{vendor}/{package}.json
- **Features**:
  - composer.json extraction
  - Packagist API compatibility
  - Vendor/package namespacing

#### 7. CocoaPods (`cocoapods/cocoapods.go`)
- **Upload**: POST with podspec JSON and tarball
- **Download**: GET /tarballs/{filename}
- **Specs**: Hierarchical spec structure
- **Features**:
  - Podspec JSON storage
  - Spec repository layout
  - pod install compatibility

#### 8. Homebrew (`brew/brew.go`)
- **Upload**: POST multipart with bottle and formula JSON
- **Download**: GET /{filename}
- **Metadata**: JSON formula API
- **Features**:
  - Bottle management
  - Formula metadata
  - brew tap compatibility

### Router Integration

**Format Router** (`internal/api/formats/router.go`)
- Mounts all format handlers under `/repository/{format}`
- Global authentication via token middleware
- Supports both read and write operations

**Management API** (`internal/api/artifacts/handler.go`)
- RESTful API for artifactadm CLI
- Endpoints:
  - GET `/api/v1/artifacts/{format}/{namespace}` - List packages
  - GET `/api/v1/artifacts/{format}/{namespace}/{package}` - List versions
  - GET `/api/v1/artifacts/{format}/{namespace}/{package}/{version}` - Get artifact
  - DELETE `/api/v1/artifacts/{format}/{namespace}/{package}/{version}` - Delete version
  - DELETE `/api/v1/artifacts/{format}/{namespace}/{package}` - Delete all versions
  - GET `/api/v1/stats` - Repository statistics

**Server Integration** (`cmd/ads-registry/cmd/serve.go`)
```go
// Multi-format artifact endpoints
formatsRouter := formats.NewRouter(store, storageProvider, tokenService, devMode)
formatsRouter.Register(r)

// Management API for CLI
artifactsAPI := artifacts.NewHandler(store)
r.Route("/api/v1/artifacts", func(api chi.Router) {
    api.Use(authMiddleware)
    api.Mount("/", artifactsAPI.Router())
})
```

### Storage Architecture

All artifacts leverage the existing OCI blob storage system:

**Storage Paths**:
```
{format}/{namespace}/{package}/{files}

Examples:
- npm/default/express/express-4.18.2.tgz
- pypi/default/requests/requests-2.28.0-py3-none-any.whl
- helm/default/mychart-1.0.0.tgz
- go/default/github.com/user/pkg/@v/v1.0.0.zip
- apt/default/pool/main/m/myapp/myapp_1.0_amd64.deb
```

**Blob Linking**:
- Artifacts reference blobs via `universal_artifact_blobs` table
- Multiple hash algorithms stored (SHA1, SHA256, SHA512)
- Efficient deduplication via digest-based storage
- Compatible with existing OCI blob storage drivers (local, S3, MinIO, OCI)

### Caching Strategy

The existing Redis caching layer (`internal/cache/cached_db.go`) transparently caches:
- Artifact metadata lookups
- Package version lists
- Blob references
- Statistics

**Cache Keys**:
```
artifact:{format}:{namespace}:{package}:{version}
artifact_list:{format}:{namespace}:{package}
artifact_stats:{format}:{namespace}
```

**TTL Configuration**:
- Metadata: Configurable (default: 1 hour)
- Blob references: Configurable (default: 24 hours)
- Statistics: Configurable (default: 5 minutes)

## artifactadm CLI Tool

### Architecture

**Command Structure** (`cmd/artifactadm/cmd/`)
```
artifactadm/
├── root.go       - CLI framework, config management
├── publish.go    - Package publishing
├── unpublish.go  - Package deletion
├── list.go       - Package listing
├── info.go       - Package information
├── scan.go       - Security scanning
├── verify.go     - Checksum verification
├── prune.go      - Version cleanup
└── stats.go      - Repository statistics
```

### Key Features

1. **Multi-Format Support**
   - Single CLI for all 8 formats
   - Format-specific upload logic
   - Auto-detection where possible

2. **Configuration Management**
   - YAML config file (~/.artifactadm.yaml)
   - Environment variable support
   - Per-format settings

3. **Authentication**
   - Token-based auth
   - Config file or environment variable
   - Secure token storage

4. **Publishing**
   - Format detection from file extension
   - Metadata extraction
   - Progress reporting
   - Error handling with retries

5. **Security**
   - Vulnerability scanning integration
   - Checksum verification
   - CI/CD pipeline support
   - Fail-on-severity thresholds

6. **Management**
   - Version pruning with policies
   - Statistics and reporting
   - Dry-run mode for safety
   - Bulk operations

### Example Usage

```bash
# Configure
cat > ~/.artifactadm.yaml <<EOF
url: https://registry.example.com
token: eyJhbGc...
format: npm
namespace: default
EOF

# Publish packages
artifactadm publish --format npm mypackage-1.0.0.tgz
artifactadm publish --format pypi dist/mypackage-1.0.0.whl
artifactadm publish --format helm mychart-1.0.0.tgz

# List and query
artifactadm list --format npm
artifactadm list --format npm express
artifactadm info --format npm express 4.18.2 --metadata

# Security
artifactadm scan --format npm express --fail-on HIGH
artifactadm verify --format pypi requests 2.28.0

# Maintenance
artifactadm prune --format npm --keep 5 --dry-run
artifactadm stats --format helm
```

## Testing Strategy

### Unit Tests

Test coverage for critical paths:

1. **Database Layer**
   - `internal/db/postgres/artifacts_test.go`
   - `internal/db/sqlite/artifacts_test.go`
   - Test CRUD operations, edge cases, concurrent access

2. **Format Handlers**
   - `internal/api/formats/{format}/{format}_test.go`
   - Test upload, download, metadata generation
   - Mock storage and database

3. **Management API**
   - `internal/api/artifacts/handler_test.go`
   - Test all REST endpoints
   - Authentication and authorization

### Integration Tests

End-to-end testing:

1. **Format Round-Trip Tests**
   - Publish package
   - Verify storage
   - Download and verify checksum
   - Query metadata
   - Delete and verify cleanup

2. **CLI Integration**
   - Test all artifactadm commands
   - Verify API interactions
   - Test error handling

3. **Multi-Tenancy Tests**
   - Namespace isolation
   - Cross-format operations
   - Concurrent access

### Performance Tests

Scalability validation:

1. **Load Testing**
   - 1000+ packages per format
   - Concurrent uploads
   - Metadata query performance

2. **Storage Efficiency**
   - Blob deduplication
   - Metadata compression
   - Index performance

## Deployment

### Prerequisites

- PostgreSQL 12+ (recommended) or SQLite 3.35+
- Storage backend (local, S3, MinIO, or OCI Object Storage)
- Redis (optional, for caching)
- Trivy (optional, for vulnerability scanning)

### Configuration

`config.json`:
```json
{
  "database": {
    "driver": "postgres",
    "dsn": "postgresql://user:pass@localhost/registry"
  },
  "storage": {
    "driver": "s3",
    "s3": {
      "bucket": "artifacts",
      "region": "us-east-1"
    }
  },
  "redis": {
    "enabled": true,
    "address": "localhost:6379"
  }
}
```

### Running

```bash
# Start server
./ads-registry serve --config config.json

# Install CLI
go install ./cmd/artifactadm

# Configure CLI
artifactadm --url https://registry.example.com --token TOKEN list --format npm
```

### Docker Deployment

```dockerfile
FROM golang:1.21 AS builder
WORKDIR /build
COPY . .
RUN go build -o ads-registry ./cmd/ads-registry
RUN go build -o artifactadm ./cmd/artifactadm

FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates
COPY --from=builder /build/ads-registry /usr/local/bin/
COPY --from=builder /build/artifactadm /usr/local/bin/
EXPOSE 5005
CMD ["ads-registry", "serve"]
```

## Monitoring

### Metrics

Prometheus metrics exposed on `/metrics`:
- `artifact_uploads_total{format}` - Total uploads per format
- `artifact_downloads_total{format}` - Total downloads per format
- `artifact_storage_bytes{format}` - Storage usage per format
- `artifact_versions_total{format}` - Version count per format

### Logging

Structured logging with format-specific context:
- Upload events with package details
- Download events with client info
- Security scan results
- Prune operations

### Alerts

Recommended alerts:
- High vulnerability count
- Storage usage thresholds
- Failed uploads
- Authentication failures

## Security Considerations

### Authentication

- Token-based authentication for all operations
- Scope-based authorization (read/write/delete)
- LDAP/OIDC integration available

### Integrity

- SHA256 checksums for all artifacts
- Immutable versions (no overwrites)
- Audit logging for all operations

### Scanning

- Trivy integration for vulnerability scanning
- Scheduled scanning of all artifacts
- Policy enforcement (fail-on-severity)

### Access Control

- Namespace-based isolation
- Format-specific permissions
- Read-only mirror support

## Migration Guide

### From npm Registry

```bash
# Export packages
npm pack mypackage
artifactadm publish --format npm mypackage-1.0.0.tgz

# Update .npmrc
npm config set registry https://registry.example.com/repository/npm/
```

### From PyPI

```bash
# Export packages
python setup.py sdist bdist_wheel

# Publish
artifactadm publish --format pypi dist/mypackage-1.0.0.whl

# Update pip config
pip config set global.index-url https://registry.example.com/repository/pypi/simple/
```

### From Artifactory/Nexus

Use migration scripts to bulk-import:
```bash
# Export from old registry
curl -O old-registry/npm/mypackage/-/mypackage-1.0.0.tgz

# Import to ADS Registry
artifactadm publish --format npm mypackage-1.0.0.tgz
```

## Future Enhancements

### Planned Features

1. **Additional Formats**
   - Maven (Java)
   - NuGet (.NET)
   - RubyGems
   - Cargo (Rust)
   - Docker (full OCI registry already supported)

2. **Advanced Security**
   - Signature verification (GPG, Sigstore)
   - SBOM generation and storage
   - License compliance scanning
   - Secret detection in packages

3. **Repository Features**
   - Package mirroring from public registries
   - Automatic dependency resolution
   - Build cache integration
   - Artifact promotion workflows

4. **Management**
   - Web UI for artifact management
   - Webhook notifications
   - Usage analytics and reporting
   - Quota management per namespace

5. **Performance**
   - CDN integration
   - Edge caching
   - Parallel upload/download
   - Delta updates

## Support

### Documentation

- Format-specific guides: `/docs/formats/`
- API reference: `/docs/api/`
- CLI reference: `/docs/ARTIFACTADM.md`

### Troubleshooting

Common issues and solutions documented in:
- `/docs/TROUBLESHOOTING.md`
- `/docs/FAQ.md`

### Community

- GitHub Issues: Report bugs and request features
- Discussions: Ask questions and share solutions
- Wiki: Community-contributed guides

## License

Copyright (c) 2024 After Dark Systems, LLC. All rights reserved.

This implementation extends the ADS Registry to support universal artifact management across multiple package formats, providing enterprise-grade reliability, security, and performance.
