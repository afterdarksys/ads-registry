# ADS Registry Migration Toolkit

A comprehensive CLI tool for migrating and inspecting OCI container images between registries.

## Features

- 🚀 **Single Image Migration** - Copy individual images between registries
- 📦 **Bulk Repository Migration** - Migrate all tags from a repository with parallel transfers
- 📋 **Batch Migration** - Migrate multiple images from a list file
- 🗂️ **Catalog Migration** - Discover and migrate entire registry catalogs
- 📊 **Image Inspection** - Count tags, calculate sizes, extract to tarballs
- ⚡ **Parallel Workers** - Configurable concurrent transfers for maximum throughput
- 🔒 **Authentication** - Auto-detects credentials from Docker config
- 🛡️ **Insecure Mode** - Support for self-signed certificates and HTTP registries
- 🔍 **Dry Run** - Preview migrations before executing
- ✅ **Error Handling** - Continue on error mode for resilient batch operations

## Installation

### From Source

```bash
cd migrate-registry
go build -o migrate-registry .
sudo mv migrate-registry /usr/local/bin/
```

### Usage

```bash
migrate-registry --help
```

## Commands

### Single Image Migration

Migrate a single image between registries:

```bash
# Basic migration
migrate-registry migrate docker.io/library/nginx:latest registry.example.com/nginx:latest

# With insecure registry (self-signed cert)
migrate-registry migrate --insecure source.io/image:tag dest.io/image:tag

# Verbose output
migrate-registry migrate -v source.io/image:tag dest.io/image:tag
```

### Bulk Repository Migration

Migrate all tags from a source repository to a destination:

```bash
# Migrate all tags from Docker Hub nginx to your registry
migrate-registry bulk-migrate-repo docker.io/library/nginx registry.example.com/nginx

# With 10 parallel workers
migrate-registry bulk-migrate-repo docker.io/library/nginx registry.example.com/nginx --workers 10

# Dry run to preview what would be migrated
migrate-registry bulk-migrate-repo source.io/repo dest.io/repo --dry-run

# Continue even if some tags fail
migrate-registry bulk-migrate-repo source.io/repo dest.io/repo --continue-on-error
```

**Options:**
- `--workers` / `-w` - Number of parallel workers (default: 5)
- `--dry-run` - Preview migration without executing
- `--continue-on-error` - Keep migrating even if some tags fail

### Bulk List Migration

Migrate multiple images specified in a text file:

```bash
# Create migration list
cat > migrations.txt <<EOF
docker.io/library/nginx:latest registry.example.com/nginx:latest
docker.io/library/redis:7 registry.example.com/redis:7
docker.io/library/postgres:15 registry.example.com/postgres:15
# Lines starting with # are comments
EOF

# Migrate from file
migrate-registry bulk-migrate-list migrations.txt

# With 10 parallel workers
migrate-registry bulk-migrate-list migrations.txt --workers 10

# Dry run
migrate-registry bulk-migrate-list migrations.txt --dry-run
```

**File Format:**
- One image pair per line: `source_image destination_image`
- Lines starting with `#` are comments
- Empty lines are ignored
- Space or tab-separated values

**Options:**
- `--workers` / `-w` - Number of parallel workers (default: 5)
- `--dry-run` - Preview migration without executing
- `--continue-on-error` - Keep migrating even if some images fail

### Bulk Catalog Migration

Discover and migrate multiple repositories from a registry catalog:

```bash
# Migrate all repositories from source to destination
migrate-registry bulk-migrate-catalog source.io dest.io

# Migrate with prefix filter (only repos starting with "myorg/")
migrate-registry bulk-migrate-catalog source.io dest.io --repo-prefix myorg/

# Migrate and transform prefix (myorg/* → neworg/*)
migrate-registry bulk-migrate-catalog source.io dest.io --repo-prefix myorg/ --dest-prefix neworg/

# With 5 parallel workers and continue on error
migrate-registry bulk-migrate-catalog source.io dest.io --workers 5 --continue-on-error

# Limit to first 50 repositories
migrate-registry bulk-migrate-catalog source.io dest.io --max-repos 50

# Skip certain tags during migration
migrate-registry bulk-migrate-catalog source.io dest.io --skip-tags latest,temp,dev
```

**Options:**
- `--workers` / `-w` - Number of parallel repository workers (default: 3)
- `--repo-prefix` - Only migrate repositories with this prefix
- `--dest-prefix` - Replace repo prefix in destination (requires --repo-prefix)
- `--max-repos` - Maximum number of repositories to migrate (0 = unlimited)
- `--skip-tags` - Comma-separated list of tags to skip
- `--dry-run` - Preview migration without executing
- `--continue-on-error` - Keep migrating even if some repositories fail

**Note:** Catalog API support varies by registry. Some registries may not implement the catalog endpoint.

### Image Inspection

#### Count Tags

Count the number of tags in a repository:

```bash
# Count tags
migrate-registry count registry.example.com/nginx

# Verbose: list all tags
migrate-registry count registry.example.com/nginx -v
```

#### Calculate Size

Calculate the total size of an image including layers, config, and manifest:

```bash
migrate-registry size registry.example.com/nginx:latest
# Output: Total size for registry.example.com/nginx:latest: 142 MB (142384512 bytes)
```

#### Extract to Tarball

Extract an image and save it as an OCI tarball for offline transfer:

```bash
# Extract image to tarball
migrate-registry extract registry.example.com/nginx:latest nginx.tar

# Extract with verbose output
migrate-registry extract -v registry.example.com/nginx:latest nginx.tar
```

## Authentication

The toolkit automatically uses credentials from your Docker configuration:

```bash
# Login to source registry
docker login source.io
Username: myuser
Password: ******

# Login to destination registry
docker login registry.example.com
Username: admin
Password: ******

# Now migrate without additional authentication
migrate-registry migrate source.io/image:tag registry.example.com/image:tag
```

Credentials are stored in `~/.docker/config.json` and automatically detected.

## Common Migration Scenarios

### Scenario 1: Migrate from Docker Hub to ADS Registry

```bash
# Single image
migrate-registry migrate docker.io/library/nginx:latest registry.example.com/nginx:latest

# All nginx tags
migrate-registry bulk-migrate-repo docker.io/library/nginx registry.example.com/nginx --workers 10
```

### Scenario 2: Migrate Entire Namespace from Oracle OCI to ADS Registry

```bash
# Migrate all repos in "mycompany" namespace
migrate-registry bulk-migrate-catalog \
  us-ashburn-1.ocir.io \
  registry.example.com \
  --repo-prefix mycompany/ \
  --dest-prefix company/ \
  --workers 5 \
  --continue-on-error
```

### Scenario 3: Cross-Cloud Migration (AWS ECR → GCP Artifact Registry)

```bash
# Login to both registries first
aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-east-1.amazonaws.com

gcloud auth configure-docker us-docker.pkg.dev

# Migrate catalog
migrate-registry bulk-migrate-catalog \
  123456789012.dkr.ecr.us-east-1.amazonaws.com \
  us-docker.pkg.dev/my-project \
  --workers 10
```

### Scenario 4: Batch Migration from File

```bash
# Create migration list with images from different registries
cat > enterprise-images.txt <<EOF
# Database images
docker.io/library/postgres:15 registry.example.com/db/postgres:15
docker.io/library/mysql:8 registry.example.com/db/mysql:8

# Application images
ghcr.io/company/app:v1.0.0 registry.example.com/apps/main:v1.0.0
ghcr.io/company/app:v1.1.0 registry.example.com/apps/main:v1.1.0

# Monitoring
quay.io/prometheus/prometheus:latest registry.example.com/monitoring/prometheus:latest
EOF

# Migrate with 15 workers and continue on error
migrate-registry bulk-migrate-list enterprise-images.txt --workers 15 --continue-on-error
```

### Scenario 5: Disaster Recovery (Extract Critical Images)

```bash
# Extract production images to tarballs
migrate-registry extract registry.example.com/app:v2.5.0 app-v2.5.0.tar
migrate-registry extract registry.example.com/db:v1.2.3 db-v1.2.3.tar

# Transfer tarballs to backup location
# Later, load them back with: docker load -i app-v2.5.0.tar
```

## Performance Tips

1. **Adjust Workers**: Increase `--workers` for faster migrations, but be mindful of:
   - Network bandwidth limitations
   - Source/destination registry rate limits
   - System resources (memory, file descriptors)

2. **Use Continue-on-Error**: For large migrations, use `--continue-on-error` to avoid stopping on transient failures

3. **Dry Run First**: Always test with `--dry-run` before large migrations

4. **Monitor Progress**: Use `-v` (verbose) flag to see detailed progress

5. **Catalog Limitations**: Some registries don't support the catalog API. Use bulk-migrate-list with a manually created list instead.

## Error Handling

The toolkit provides comprehensive error handling:

- **Network Failures**: Automatic retries through crane library
- **Authentication Errors**: Clear error messages with authentication hints
- **Missing Images**: Reports which images couldn't be found
- **Continue on Error**: `--continue-on-error` flag allows batch operations to complete despite individual failures
- **Summary Reports**: Detailed success/failure counts at the end of bulk operations

## Registry Compatibility

Tested and compatible with:

- ✅ **Docker Hub** (registry-1.docker.io)
- ✅ **AWS ECR** (*.dkr.ecr.*.amazonaws.com)
- ✅ **Google Artifact Registry** (*-docker.pkg.dev)
- ✅ **Oracle OCI Registry** (*.ocir.io)
- ✅ **Azure Container Registry** (*.azurecr.io)
- ✅ **GitHub Container Registry** (ghcr.io)
- ✅ **Quay.io** (quay.io)
- ✅ **Harbor** (self-hosted)
- ✅ **JFrog Artifactory** (self-hosted)
- ✅ **ADS Registry** (self-hosted)

## Limitations

1. **Catalog API**: Not all registries implement the v2 catalog endpoint (required for `bulk-migrate-catalog`)
2. **Rate Limits**: Respect source/destination registry rate limits by adjusting `--workers`
3. **Authentication**: Some registries require special authentication flows (use `docker login` first)
4. **Large Images**: Very large images may require significant bandwidth and time

## Troubleshooting

### Authentication Fails

```bash
# Ensure you're logged in to both registries
docker login source.io
docker login dest.io

# Verify credentials are stored
cat ~/.docker/config.json
```

### Self-Signed Certificates

```bash
# Use --insecure flag
migrate-registry migrate --insecure https://registry.local/image:tag dest.io/image:tag
```

### Catalog API Not Supported

```bash
# Error: "listing repositories: ... catalog API"
# Solution: Create a manual list and use bulk-migrate-list instead

# List repos manually (if registry provides this)
curl -u user:pass https://registry/v2/_catalog | jq -r '.repositories[]' > repos.txt

# Transform to migration list format
while read repo; do
  echo "source.io/$repo dest.io/$repo"
done < repos.txt > migrations.txt

# Migrate
migrate-registry bulk-migrate-list migrations.txt
```

### Rate Limiting

```bash
# Reduce workers to avoid rate limits
migrate-registry bulk-migrate-repo source/repo dest/repo --workers 2
```

## Build Information

Built with:
- [google/go-containerregistry](https://github.com/google/go-containerregistry) - OCI image manipulation
- [spf13/cobra](https://github.com/spf13/cobra) - CLI framework
- [dustin/go-humanize](https://github.com/dustin/go-humanize) - Human-readable output

## License

Part of the ADS Container Registry project.

## Support

For issues and questions:
- GitHub Issues: [ads-registry/issues](https://github.com/ryan/ads-registry/issues)
- Documentation: [Main README](../README.md)
