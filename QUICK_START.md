# ADS Registry - Quick Start Guide

## Registry Status

**Registry is RUNNING on port 5050**

Check status: `curl http://localhost:5050/health/live`

## Supported Formats

The registry now supports **9 artifact types**:

1. **Docker/OCI** - Container images (`/v2/`)
2. **npm** - JavaScript packages (`/repository/npm/`)
3. **PyPI** - Python packages (`/repository/pypi/`)
4. **Helm** - Kubernetes charts (`/repository/helm/`)
5. **Go** - Go modules (`/repository/go/`)
6. **APT** - Debian packages (`/repository/apt/`)
7. **Composer** - PHP packages (`/repository/composer/`)
8. **CocoaPods** - iOS/macOS packages (`/repository/cocoapods/`)
9. **Homebrew** - macOS packages (`/repository/brew/`)

## Quick Commands

### Registry Management

```bash
# View logs
tail -f registry.log

# Stop registry
kill $(cat registry.pid)

# Start registry
nohup ./cmd/ads-registry/ads-registry serve -c config.json > registry.log 2>&1 & echo $! > registry.pid

# Check status
ps aux | grep ads-registry | grep -v grep
```

### Test Endpoints

```bash
# Health check
curl http://localhost:5050/health/live

# List npm packages
curl http://localhost:5050/api/v1/artifacts/npm/default

# List PyPI packages
curl http://localhost:5050/api/v1/artifacts/pypi/default

# Docker registry
curl http://localhost:5050/v2/
```

### Database Queries

```bash
# Connect to database
psql -h localhost -p 5432 -U ads_user -d ads_registry

# View tables
\dt universal_*

# Count packages
SELECT format, COUNT(*) FROM universal_artifacts GROUP BY format;
```

## Publishing Packages

### npm
```bash
npm config set registry http://localhost:5050/repository/npm/
npm publish
```

### PyPI
```bash
twine upload --repository-url http://localhost:5050/repository/pypi/ dist/*
```

### Helm
```bash
helm push my-chart.tgz oci://localhost:5050/repository/helm/
```

### Docker
```bash
docker tag myimage:latest localhost:5050/myimage:latest
docker push localhost:5050/myimage:latest
```

## File Locations

```
/Users/ryan/development/ads-registry/
├── cmd/ads-registry/ads-registry    # Main binary
├── cmd/artifactadm/artifactadm      # CLI tool
├── config.json                       # Configuration
├── registry.log                      # Logs
├── registry.pid                      # Process ID
└── data/blobs/                       # Storage
```

## Important Notes

1. **Developer Mode Active** - Authentication is bypassed for testing
2. **Port 5050** - Registry listening on all interfaces
3. **Database** - PostgreSQL at localhost:5432/ads_registry
4. **Redis** - Caching enabled at localhost:6379

## Troubleshooting

### Registry not responding?
```bash
ps aux | grep ads-registry
tail -100 registry.log
```

### Database issues?
```bash
psql -h localhost -p 5432 -U ads_user -d ads_registry -c "SELECT version();"
```

### Port already in use?
```bash
lsof -i :5050  # May not work on all systems
# Kill process if needed, then restart
```

## Next Steps

1. Test package publishing with each format
2. Enable authentication for production
3. Configure TLS certificates
4. Set up monitoring/alerting
5. Configure backups

For complete documentation, see `DEPLOYMENT_COMPLETE.md`

---
Status: OPERATIONAL ✅
Version: 1.1.0
Date: April 5, 2026
