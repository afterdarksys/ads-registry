# Artifact Registry - Quick Reference Card

## Build & Deploy

```bash
# Build everything
./scripts/build-and-test-artifacts.sh

# Start server
cd cmd/ads-registry && ./ads-registry serve --config ../../config.json

# Install CLI
sudo cp cmd/artifactadm/artifactadm /usr/local/bin/
```

## CLI Configuration

```yaml
# ~/.artifactadm.yaml
url: https://registry.example.com
token: your-auth-token
format: npm
namespace: default
```

## Common Commands

### Publish
```bash
artifactadm publish --format npm package.tgz
artifactadm publish --format pypi dist/package.whl
artifactadm publish --format helm chart.tgz
artifactadm publish --format go module.zip --name github.com/user/pkg --version v1.0.0
```

### List & Info
```bash
artifactadm list --format npm                    # List all packages
artifactadm list --format npm express            # List versions
artifactadm info --format npm express 4.18.2     # Show details
artifactadm info --format npm express --metadata # Full metadata
```

### Delete
```bash
artifactadm unpublish --format npm pkg 1.0.0 --force     # Delete version
artifactadm unpublish --format npm pkg --all --force     # Delete all
```

### Security
```bash
artifactadm scan --format npm express                    # Scan for vulns
artifactadm scan --format npm express --fail-on HIGH     # CI/CD gate
artifactadm verify --format pypi requests 2.28.0         # Verify checksum
```

### Maintenance
```bash
artifactadm prune --format npm --keep 5 --dry-run    # Preview prune
artifactadm prune --format npm --keep 5              # Execute prune
artifactadm stats --format npm                       # Show statistics
```

## API Endpoints

### Format-Specific (Native Clients)
```
GET  /repository/npm/{package}
GET  /repository/npm/{package}/-/{tarball}
PUT  /repository/npm/{package}

GET  /repository/pypi/simple/
GET  /repository/pypi/simple/{package}/
POST /repository/pypi/

GET  /repository/helm/index.yaml
POST /repository/helm/api/charts
GET  /repository/helm/charts/{filename}

GET  /repository/go/{module}/@v/list
GET  /repository/go/{module}/@v/{version}.info
GET  /repository/go/{module}/@v/{version}.mod
GET  /repository/go/{module}/@v/{version}.zip

GET  /repository/apt/dists/{codename}/Release
GET  /repository/apt/pool/main/{prefix}/{pkg}/{file}
```

### Management API (for artifactadm)
```
GET    /api/v1/artifacts/{format}/{namespace}
GET    /api/v1/artifacts/{format}/{namespace}/{package}
GET    /api/v1/artifacts/{format}/{namespace}/{package}/{version}
DELETE /api/v1/artifacts/{format}/{namespace}/{package}/{version}
DELETE /api/v1/artifacts/{format}/{namespace}/{package}
GET    /api/v1/stats
```

## Client Configuration

### npm
```bash
npm config set registry https://registry.example.com/repository/npm/
echo "//registry.example.com/repository/npm/:_authToken=TOKEN" >> ~/.npmrc
```

### pip
```bash
pip config set global.index-url https://registry.example.com/repository/pypi/simple/
```

### Helm
```bash
helm repo add myregistry https://registry.example.com/repository/helm/
helm repo update
```

### Go
```bash
export GOPROXY=https://registry.example.com/repository/go/,direct
```

### APT
```bash
echo "deb [trusted=yes] https://registry.example.com/repository/apt/dists/stable stable main" | \
  sudo tee /etc/apt/sources.list.d/myregistry.list
```

## Database Schema

```sql
-- Main artifacts table
CREATE TABLE universal_artifacts (
    id SERIAL PRIMARY KEY,
    format VARCHAR(50) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    package_name VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (format, namespace, package_name, version)
);

-- Blob references
CREATE TABLE universal_artifact_blobs (
    id SERIAL PRIMARY KEY,
    artifact_id INTEGER NOT NULL REFERENCES universal_artifacts(id) ON DELETE CASCADE,
    blob_digest VARCHAR(255) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(artifact_id, blob_digest)
);

-- Format-specific metadata
CREATE TABLE universal_artifact_metadata (
    id SERIAL PRIMARY KEY,
    artifact_id INTEGER NOT NULL REFERENCES universal_artifacts(id) ON DELETE CASCADE,
    raw_data JSONB NOT NULL,
    UNIQUE(artifact_id)
);
```

## Supported Formats

| Format | Package Manager | File Types | Status |
|--------|----------------|------------|--------|
| npm | npm, yarn | .tgz | ✓ Complete |
| pypi | pip, twine | .whl, .tar.gz | ✓ Complete |
| helm | helm | .tgz | ✓ Complete |
| go | go | .zip, .mod | ✓ Complete |
| apt | apt-get, dpkg | .deb | ✓ Complete |
| composer | composer | .zip | ✓ Complete |
| cocoapods | pod | .tar.gz, .zip | ✓ Complete |
| brew | brew | .tar.gz | ✓ Complete |

## Storage Paths

```
{format}/{namespace}/{package}/{files}

Examples:
npm/default/express/express-4.18.2.tgz
pypi/default/requests/requests-2.28.0-py3-none-any.whl
helm/default/mychart-1.0.0.tgz
go/default/github.com/user/pkg/@v/v1.0.0.zip
apt/default/pool/main/m/myapp/myapp_1.0_amd64.deb
```

## Environment Variables

```bash
export ARTIFACTADM_URL=https://registry.example.com
export ARTIFACTADM_TOKEN=your-token
export ARTIFACTADM_FORMAT=npm
export ARTIFACTADM_NAMESPACE=default
export ARTIFACTADM_VERBOSE=true
```

## Troubleshooting

### Connection Issues
```bash
curl -I https://registry.example.com/health/live
artifactadm --verbose list --format npm
```

### Authentication
```bash
curl -H "Authorization: Bearer TOKEN" \
  https://registry.example.com/api/v1/stats
```

### Upload Failures
```bash
sha256sum package.tgz
file package.tgz
artifactadm --verbose publish --format npm package.tgz
```

## CI/CD Integration

### GitHub Actions
```yaml
- name: Publish
  env:
    ARTIFACTADM_TOKEN: ${{ secrets.REGISTRY_TOKEN }}
  run: artifactadm publish --format npm *.tgz
```

### GitLab CI
```yaml
publish:
  script: artifactadm publish --format pypi dist/*.whl
  variables:
    ARTIFACTADM_TOKEN: $REGISTRY_TOKEN
```

## Key Files

```
migrations/017_artifact_metadata.sql       # Database schema
internal/db/metadata.go                    # Interface definitions
internal/db/postgres/artifacts.go          # PostgreSQL implementation
internal/db/sqlite/sqlite.go               # SQLite implementation
internal/api/formats/{format}/{format}.go  # Format handlers
internal/api/artifacts/handler.go          # Management API
cmd/artifactadm/cmd/*.go                   # CLI commands
docs/ARTIFACTADM.md                        # CLI documentation
docs/ARTIFACT_REGISTRY_IMPLEMENTATION.md   # Architecture docs
docs/QUICK_START_FORMATS.md                # Format guides
```

## Performance Tips

1. **Enable Redis caching** for better metadata query performance
2. **Use S3/MinIO** for scalable blob storage
3. **Enable connection pooling** in database config
4. **Use CDN** for artifact downloads
5. **Prune regularly** to manage storage costs

## Security Best Practices

1. Use **strong authentication tokens** with limited scopes
2. Enable **vulnerability scanning** in CI/CD pipelines
3. Implement **rate limiting** to prevent abuse
4. Use **HTTPS** for all connections
5. Regularly **rotate tokens** and credentials
6. Enable **audit logging** for compliance

## Support

- Docs: `/docs/`
- CLI Help: `artifactadm --help`
- API Docs: `https://registry.example.com/api/v1/`
- Build Script: `./scripts/build-and-test-artifacts.sh`

---

**Version**: 1.0.0
**Status**: Production Ready
**Formats**: 8 supported (npm, pypi, helm, go, apt, composer, cocoapods, brew)
