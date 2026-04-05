# artifactadm - Universal Artifact Registry Management Tool

`artifactadm` is a comprehensive CLI tool for managing multi-format artifacts in the ADS Registry. It provides a unified interface for publishing, managing, and securing packages across 8 different artifact formats.

## Supported Formats

- **npm** - Node.js packages
- **pypi** - Python packages
- **helm** - Kubernetes Helm charts
- **go** - Go modules
- **apt** - Debian packages
- **composer** - PHP packages
- **cocoapods** - iOS/macOS dependencies
- **brew** - Homebrew formulas/bottles

## Installation

```bash
# Build from source
cd cmd/artifactadm
go build -o artifactadm

# Install to PATH
sudo cp artifactadm /usr/local/bin/
```

## Quick Start

### 1. Configure Connection

Create `~/.artifactadm.yaml`:

```yaml
url: https://registry.example.com
token: your-auth-token-here
format: npm  # default format
namespace: default
verbose: false
```

Or use environment variables:

```bash
export ARTIFACTADM_URL=https://registry.example.com
export ARTIFACTADM_TOKEN=your-token
```

### 2. Publish a Package

```bash
# npm package
artifactadm publish --format npm ./mypackage-1.0.0.tgz

# Python package
artifactadm publish --format pypi ./dist/mypackage-1.0.0-py3-none-any.whl

# Helm chart
artifactadm publish --format helm ./mychart-1.0.0.tgz

# Go module
artifactadm publish --format go ./mymodule-v1.0.0.zip --name github.com/user/mymodule --version v1.0.0

# Debian package
artifactadm publish --format apt ./mypackage_1.0.0_amd64.deb
```

### 3. List Packages

```bash
# List all npm packages
artifactadm list --format npm

# List versions of specific package
artifactadm list --format npm express

# JSON output
artifactadm list --format pypi --json
```

### 4. Get Package Info

```bash
# Show latest version info
artifactadm info --format npm express

# Show specific version
artifactadm info --format npm express 4.18.2

# Show full metadata
artifactadm info --format pypi requests --metadata

# Show blob details
artifactadm info --format helm mychart --blobs
```

## Commands

### Package Management

#### publish

Publish an artifact to the registry.

```bash
artifactadm publish [flags] <package-file>

Flags:
  --format string       Artifact format (required)
  --namespace string    Namespace (default "default")
  --name string         Package name (auto-detected)
  --version string      Package version (auto-detected)
  --metadata string     Additional metadata as JSON
```

#### unpublish

Remove a package or version from the registry.

```bash
artifactadm unpublish [flags] <package-name> [version]

Flags:
  --format string    Artifact format (required)
  --force           Skip confirmation prompt
  --all             Delete all versions
```

**WARNING**: This action is irreversible!

#### list

List packages or package versions.

```bash
artifactadm list [flags] [package-name]

Flags:
  --format string    Artifact format (required)
  --all             List all packages (not just names)
  --limit int       Maximum results (default 100)
  --json            Output as JSON
```

#### info

Show detailed package information.

```bash
artifactadm info [flags] <package-name> [version]

Flags:
  --format string    Artifact format (required)
  --metadata        Show full metadata
  --blobs           Show blob details
  --json            Output as JSON
```

### Security

#### scan

Scan package for security vulnerabilities.

```bash
artifactadm scan [flags] <package-name> [version]

Flags:
  --format string      Artifact format (required)
  --engine string      Scanner engine (trivy, static) (default "trivy")
  --severity string    Filter by severity (LOW,MEDIUM,HIGH,CRITICAL)
  --fail-on string     Fail if vulnerabilities found at severity level
  --output string      Save report to file
  --json              Output as JSON
```

Example CI/CD integration:

```bash
# Fail build on critical vulnerabilities
artifactadm scan --format npm myapp --fail-on CRITICAL || exit 1
```

#### verify

Verify package checksums and signatures.

```bash
artifactadm verify [flags] <package-name> <version>

Flags:
  --format string      Artifact format (required)
  --checksum string    Expected SHA256 checksum
  --download          Download and verify locally
```

### Repository Management

#### prune

Clean up old package versions.

```bash
artifactadm prune [flags]

Flags:
  --format string         Artifact format (required)
  --keep int             Keep N recent versions (default 5)
  --older-than string    Delete versions older than duration (e.g., 90d, 6m)
  --dry-run             Preview changes without deleting
```

Examples:

```bash
# Preview what would be deleted
artifactadm prune --format npm --keep 3 --dry-run

# Keep only 5 most recent versions
artifactadm prune --format pypi --keep 5

# Delete versions older than 90 days
artifactadm prune --format helm --older-than 90d
```

#### stats

Show repository statistics.

```bash
artifactadm stats [flags]

Flags:
  --format string    Show stats for specific format
  --json            Output as JSON
```

## Format-Specific Guides

### npm

```bash
# Publish npm package
npm pack
artifactadm publish --format npm mypackage-1.0.0.tgz

# Configure npm to use registry
npm config set registry https://registry.example.com/repository/npm/

# Install from private registry
npm install mypackage
```

### PyPI

```bash
# Publish Python package
python setup.py sdist bdist_wheel
artifactadm publish --format pypi dist/mypackage-1.0.0-py3-none-any.whl

# Configure pip to use registry
pip install --index-url https://registry.example.com/repository/pypi/simple/ mypackage
```

### Helm

```bash
# Package and publish chart
helm package ./mychart
artifactadm publish --format helm mychart-1.0.0.tgz

# Add registry as Helm repository
helm repo add myregistry https://registry.example.com/repository/helm/
helm repo update

# Install chart
helm install myapp myregistry/mychart
```

### Go Modules

```bash
# Publish Go module
artifactadm publish --format go mymodule-v1.0.0.zip \\
  --name github.com/user/mymodule \\
  --version v1.0.0

# Configure Go to use proxy
export GOPROXY=https://registry.example.com/repository/go/,direct

# Install module
go get github.com/user/mymodule@v1.0.0
```

### APT (Debian)

```bash
# Publish Debian package
artifactadm publish --format apt mypackage_1.0.0_amd64.deb

# Add repository to sources.list
echo "deb [trusted=yes] https://registry.example.com/repository/apt/dists/stable stable main" | \\
  sudo tee /etc/apt/sources.list.d/myregistry.list

# Install package
sudo apt update
sudo apt install mypackage
```

### Composer (PHP)

```bash
# Publish Composer package
artifactadm publish --format composer mypackage-1.0.0.zip

# Configure composer.json
{
  "repositories": [
    {
      "type": "composer",
      "url": "https://registry.example.com/repository/composer/"
    }
  ]
}

# Install package
composer require vendor/mypackage
```

### CocoaPods

```bash
# Publish pod
artifactadm publish --format cocoapods mypod-1.0.0.tar.gz \\
  --metadata '{"name":"MyPod","version":"1.0.0"}'

# Configure Podfile
source 'https://registry.example.com/repository/cocoapods/'

# Install pod
pod install
```

### Homebrew

```bash
# Publish bottle
artifactadm publish --format brew myformula-1.0.0.tar.gz \\
  --name myformula \\
  --version 1.0.0

# Add tap
brew tap myorg/tap https://registry.example.com/repository/brew/

# Install formula
brew install myformula
```

## Authentication

### Token-Based Authentication

Get an authentication token from your registry administrator:

```bash
# Set token in config file
artifactadm --token YOUR_TOKEN list --format npm

# Or use environment variable
export ARTIFACTADM_TOKEN=YOUR_TOKEN
```

### Token Scopes

Tokens can have different permission levels:

- **read**: List and download packages
- **write**: Publish packages
- **delete**: Remove packages
- **admin**: Full administrative access

## CI/CD Integration

### GitHub Actions

```yaml
name: Publish Package

on:
  release:
    types: [created]

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install artifactadm
        run: |
          curl -L https://github.com/your-org/artifactadm/releases/latest/download/artifactadm-linux-amd64 -o artifactadm
          chmod +x artifactadm
          sudo mv artifactadm /usr/local/bin/

      - name: Publish package
        env:
          ARTIFACTADM_TOKEN: ${{ secrets.REGISTRY_TOKEN }}
          ARTIFACTADM_URL: https://registry.example.com
        run: |
          artifactadm publish --format npm ./mypackage-*.tgz

      - name: Security scan
        run: |
          artifactadm scan --format npm mypackage --fail-on HIGH
```

### GitLab CI

```yaml
publish:
  stage: deploy
  image: golang:1.21
  script:
    - go install github.com/your-org/artifactadm@latest
    - artifactadm publish --format pypi dist/*.whl
    - artifactadm scan --format pypi mypackage --fail-on CRITICAL
  only:
    - tags
  variables:
    ARTIFACTADM_URL: https://registry.example.com
    ARTIFACTADM_TOKEN: $REGISTRY_TOKEN
```

## Troubleshooting

### Connection Issues

```bash
# Test connectivity
curl -I https://registry.example.com/health/live

# Verify authentication
artifactadm stats --verbose
```

### Upload Failures

```bash
# Check file integrity
sha256sum mypackage.tgz

# Verify format
file mypackage.tgz

# Check registry logs
artifactadm --verbose publish --format npm mypackage.tgz
```

### Permission Errors

Ensure your token has the correct scopes:

```bash
# Request token with write permissions
# Contact your registry administrator
```

## Best Practices

### 1. Version Management

- Use semantic versioning (X.Y.Z)
- Never reuse version numbers
- Keep old versions for rollback capability

### 2. Security

- Scan all packages before publishing
- Verify checksums for critical packages
- Use token rotation policies
- Implement least-privilege access

### 3. Storage Optimization

- Prune old versions regularly
- Monitor storage usage with `stats`
- Archive packages to cold storage if needed

### 4. Automation

- Integrate scanning in CI/CD pipelines
- Automate pruning with scheduled jobs
- Use fail-on thresholds to enforce quality gates

## Support

For issues and questions:

- GitHub Issues: https://github.com/your-org/ads-registry/issues
- Documentation: https://docs.example.com/ads-registry
- Enterprise Support: support@example.com

## License

Copyright (c) 2024 After Dark Systems, LLC. All rights reserved.
