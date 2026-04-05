# Quick Start Guide - All Artifact Formats

This guide provides quick setup and usage examples for all supported artifact formats in ADS Registry.

## Table of Contents

1. [npm (Node.js)](#npm-nodejs)
2. [PyPI (Python)](#pypi-python)
3. [Helm (Kubernetes)](#helm-kubernetes)
4. [Go Modules](#go-modules)
5. [APT (Debian/Ubuntu)](#apt-debianubuntu)
6. [Composer (PHP)](#composer-php)
7. [CocoaPods (iOS/macOS)](#cocoapods-iosmacos)
8. [Homebrew (macOS)](#homebrew-macos)

---

## npm (Node.js)

### Publish

```bash
# Package your module
npm pack

# Publish to registry
artifactadm publish --format npm mypackage-1.0.0.tgz
```

### Configure npm

```bash
# Set registry
npm config set registry https://registry.example.com/repository/npm/

# Login (one-time)
npm login --registry=https://registry.example.com/repository/npm/
```

### Install

```bash
npm install mypackage
```

### .npmrc Configuration

```ini
registry=https://registry.example.com/repository/npm/
//registry.example.com/repository/npm/:_authToken=${NPM_TOKEN}
```

---

## PyPI (Python)

### Publish

```bash
# Build distribution
python setup.py sdist bdist_wheel

# Publish
artifactadm publish --format pypi dist/mypackage-1.0.0-py3-none-any.whl
```

### Configure pip

```bash
# Install from registry
pip install --index-url https://registry.example.com/repository/pypi/simple/ mypackage
```

### pip.conf Configuration

```ini
[global]
index-url = https://registry.example.com/repository/pypi/simple/
trusted-host = registry.example.com
```

### Using Twine (Alternative)

```bash
# Configure .pypirc
cat > ~/.pypirc <<EOF
[distutils]
index-servers = ads-registry

[ads-registry]
repository = https://registry.example.com/repository/pypi/
username = __token__
password = your-token-here
EOF

# Upload with twine
twine upload -r ads-registry dist/*
```

---

## Helm (Kubernetes)

### Publish

```bash
# Package chart
helm package ./mychart

# Publish
artifactadm publish --format helm mychart-1.0.0.tgz
```

### Add Repository

```bash
# Add registry as Helm repo
helm repo add myregistry https://registry.example.com/repository/helm/ \\
  --username admin --password your-token

# Update repo index
helm repo update
```

### Install Chart

```bash
# Search charts
helm search repo myregistry

# Install chart
helm install myapp myregistry/mychart --version 1.0.0
```

---

## Go Modules

### Publish

```bash
# Create module archive
git archive -o mymodule-v1.0.0.zip --prefix=mymodule@v1.0.0/ v1.0.0

# Publish
artifactadm publish --format go mymodule-v1.0.0.zip \\
  --name github.com/user/mymodule \\
  --version v1.0.0
```

### Configure Go

```bash
# Set GOPROXY
export GOPROXY=https://registry.example.com/repository/go/,direct

# Or in go.env
go env -w GOPROXY=https://registry.example.com/repository/go/,direct
```

### Use Module

```bash
# Get module
go get github.com/user/mymodule@v1.0.0

# Import in code
import "github.com/user/mymodule"
```

### Private Module Authentication

```bash
# Configure .netrc for authentication
cat > ~/.netrc <<EOF
machine registry.example.com
login token
password your-token-here
EOF

chmod 600 ~/.netrc

# Mark as private
go env -w GOPRIVATE=github.com/user/*
```

---

## APT (Debian/Ubuntu)

### Publish

```bash
# Build package
dpkg-deb --build mypackage

# Publish
artifactadm publish --format apt mypackage_1.0.0_amd64.deb
```

### Add Repository

```bash
# Add GPG key (if signed)
curl -fsSL https://registry.example.com/gpg.key | sudo apt-key add -

# Add repository to sources.list
echo "deb [trusted=yes] https://registry.example.com/repository/apt/dists/stable stable main" | \\
  sudo tee /etc/apt/sources.list.d/ads-registry.list
```

### Install Package

```bash
# Update package index
sudo apt update

# Install package
sudo apt install mypackage
```

### For Authenticated Access

```bash
# Add credentials to sources.list
echo "deb https://token:YOUR_TOKEN@registry.example.com/repository/apt/dists/stable stable main" | \\
  sudo tee /etc/apt/sources.list.d/ads-registry.list
```

---

## Composer (PHP)

### Publish

```bash
# Create ZIP archive with composer.json
zip -r mypackage-1.0.0.zip . -x "*.git*" -x "vendor/*"

# Publish
artifactadm publish --format composer mypackage-1.0.0.zip
```

### Configure Composer

Edit `composer.json`:

```json
{
  "repositories": [
    {
      "type": "composer",
      "url": "https://registry.example.com/repository/composer/"
    }
  ],
  "require": {
    "vendor/mypackage": "^1.0"
  }
}
```

### Install Package

```bash
composer install
```

### Authentication

```bash
# Configure auth
composer config --global http-basic.registry.example.com token YOUR_TOKEN
```

---

## CocoaPods (iOS/macOS)

### Publish

```bash
# Create tarball
tar czf MyPod-1.0.0.tar.gz MyPod/

# Create podspec JSON
cat > podspec.json <<EOF
{
  "name": "MyPod",
  "version": "1.0.0",
  "summary": "My awesome pod",
  "homepage": "https://example.com/mypod",
  "license": "MIT",
  "authors": { "Author": "author@example.com" },
  "source": {
    "http": "https://registry.example.com/repository/cocoapods/tarballs/MyPod-1.0.0.tar.gz"
  },
  "platforms": {
    "ios": "11.0"
  },
  "source_files": "MyPod/**/*.{h,m,swift}"
}
EOF

# Publish
artifactadm publish --format cocoapods MyPod-1.0.0.tar.gz \\
  --metadata "$(cat podspec.json)"
```

### Configure Podfile

```ruby
source 'https://registry.example.com/repository/cocoapods/'

platform :ios, '11.0'
use_frameworks!

target 'MyApp' do
  pod 'MyPod', '~> 1.0'
end
```

### Install Pod

```bash
pod install
```

---

## Homebrew (macOS)

### Publish

```bash
# Create bottle tarball
tar czf myformula-1.0.0.tar.gz myformula/

# Create formula JSON (optional)
cat > formula.json <<EOF
{
  "name": "myformula",
  "version": "1.0.0",
  "desc": "My awesome formula",
  "homepage": "https://example.com/myformula",
  "bottle": {
    "stable": {
      "files": {
        "all": {
          "url": "https://registry.example.com/repository/brew/myformula-1.0.0.tar.gz",
          "sha256": "abc123..."
        }
      }
    }
  }
}
EOF

# Publish
artifactadm publish --format brew myformula-1.0.0.tar.gz \\
  --name myformula \\
  --version 1.0.0 \\
  --metadata "$(cat formula.json)"
```

### Add Tap

```bash
# Add custom tap
brew tap myorg/tap https://registry.example.com/repository/brew/
```

### Install Formula

```bash
brew install myformula
```

---

## Common Operations

### List Packages

```bash
# List all packages for a format
artifactadm list --format npm

# List versions of specific package
artifactadm list --format npm express
```

### Get Package Info

```bash
# Show package details
artifactadm info --format npm express 4.18.2

# Show with metadata
artifactadm info --format npm express --metadata
```

### Delete Package

```bash
# Delete specific version
artifactadm unpublish --format npm express 4.18.2 --force

# Delete all versions
artifactadm unpublish --format npm express --all --force
```

### Security Scanning

```bash
# Scan for vulnerabilities
artifactadm scan --format npm express

# Fail on high severity
artifactadm scan --format npm express --fail-on HIGH
```

### Maintenance

```bash
# Show statistics
artifactadm stats --format npm

# Prune old versions
artifactadm prune --format npm --keep 5

# Preview prune
artifactadm prune --format npm --keep 3 --dry-run
```

---

## Environment Variables

All formats support these environment variables:

```bash
export ARTIFACTADM_URL=https://registry.example.com
export ARTIFACTADM_TOKEN=your-token-here
export ARTIFACTADM_FORMAT=npm
export ARTIFACTADM_NAMESPACE=default
```

## Troubleshooting

### Connection Issues

```bash
# Test registry connectivity
curl -I https://registry.example.com/health/live

# Verbose output
artifactadm --verbose list --format npm
```

### Authentication Failures

```bash
# Verify token
curl -H "Authorization: Bearer YOUR_TOKEN" \\
  https://registry.example.com/api/v1/stats

# Check token scopes
# Contact registry administrator
```

### Upload Failures

```bash
# Verify file integrity
sha256sum mypackage.tgz

# Check file format
file mypackage.tgz

# Check registry logs
# Contact registry administrator
```

## CI/CD Integration Examples

### GitHub Actions

```yaml
- name: Publish to Registry
  env:
    ARTIFACTADM_TOKEN: ${{ secrets.REGISTRY_TOKEN }}
    ARTIFACTADM_URL: https://registry.example.com
  run: |
    go install github.com/your-org/artifactadm@latest
    artifactadm publish --format npm *.tgz
```

### GitLab CI

```yaml
publish:
  script:
    - artifactadm publish --format pypi dist/*.whl
  variables:
    ARTIFACTADM_URL: https://registry.example.com
    ARTIFACTADM_TOKEN: $REGISTRY_TOKEN
```

### Jenkins

```groovy
stage('Publish') {
  environment {
    ARTIFACTADM_TOKEN = credentials('registry-token')
    ARTIFACTADM_URL = 'https://registry.example.com'
  }
  steps {
    sh 'artifactadm publish --format helm *.tgz'
  }
}
```

## Support

- Documentation: https://docs.example.com/ads-registry
- Issues: https://github.com/your-org/ads-registry/issues
- CLI Help: `artifactadm --help`
- Format Help: `artifactadm publish --help`
