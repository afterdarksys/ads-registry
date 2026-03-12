# Pull-Through Cache (Registry Proxy)

**Cache images from Docker Hub, GCR, GitHub, and other registries**

---

## Overview

The Pull-Through Cache feature allows your ADS Container Registry to act as a **transparent proxy** for upstream registries like Docker Hub, Google Container Registry (GCR), GitHub Container Registry (GHCR), and others.

**Benefits:**
- 🚀 **Faster pulls** - Images cached locally after first pull
- 💰 **Reduced bandwidth** - Pull from Docker Hub without hitting rate limits
- 🔒 **Air-gapped deployments** - Cache all dependencies locally
- 📦 **Centralized registry** - Single endpoint for all images
- 🔄 **Automatic updates** - Configurable cache TTL

---

## How It Works

```
Docker Pull → Registry Proxy → Check Cache → Upstream → Cache → Client
                  ↓                 ↓           ↓        ↓        ↓
              /v2/proxy/...    Local Storage   Docker   Save    Stream
                               (Cache HIT)      Hub     Locally  to Docker
                                    ↓
                               Return Cached
```

**Pull Flow:**

1. **Client pulls image** - `docker pull apps.afterdarksys.com:5005/proxy/dockerhub/library/ubuntu:latest`
2. **Registry checks cache** - Is `ubuntu:latest` already cached?
3. **Cache HIT**: Return cached manifest + blobs immediately
4. **Cache MISS**:
   - Fetch manifest from Docker Hub
   - Fetch blobs (layers) from Docker Hub
   - Stream to client while caching in background
   - Next pull is served from cache

---

## Quick Start

### 1. Enable Pull-Through Cache

Edit `config/proxy.json`:

```json
{
  "enabled": true,
  "cache_ttl_hours": 168,
  "upstreams": [
    {
      "name": "dockerhub",
      "url": "https://registry-1.docker.io",
      "mirror": true
    }
  ]
}
```

### 2. Pull Through Proxy

**Before** (direct from Docker Hub):
```bash
docker pull ubuntu:latest
```

**After** (through your registry):
```bash
docker pull apps.afterdarksys.com:5005/proxy/dockerhub/library/ubuntu:latest
```

First pull: fetches from Docker Hub + caches
Second pull: served from cache ⚡

---

## Supported Upstream Registries

### Docker Hub

```json
{
  "name": "dockerhub",
  "url": "https://registry-1.docker.io",
  "username": "<DOCKER_USERNAME>",
  "password": "<DOCKER_PAT>",
  "mirror": true
}
```

**Pull Syntax:**
```bash
# Official images (library/)
docker pull apps.afterdarksys.com:5005/proxy/dockerhub/library/ubuntu:22.04
docker pull apps.afterdarksys.com:5005/proxy/dockerhub/library/nginx:alpine

# User images
docker pull apps.afterdarksys.com:5005/proxy/dockerhub/username/repo:tag
```

**Authentication:**
- Anonymous: 100 pulls/6 hours
- Authenticated: 200 pulls/6 hours (free tier)
- Pro/Team: Unlimited pulls

### Google Container Registry (GCR)

```json
{
  "name": "gcr",
  "url": "https://gcr.io",
  "username": "_json_key",
  "password": "<GCR_SERVICE_ACCOUNT_JSON>",
  "mirror": false
}
```

**Pull Syntax:**
```bash
docker pull apps.afterdarksys.com:5005/proxy/gcr/google-samples/hello-app:1.0
docker pull apps.afterdarksys.com:5005/proxy/gcr/my-project/my-image:v2.1
```

**Setup:**
```bash
# Create service account
gcloud iam service-accounts create registry-proxy

# Grant storage.objectViewer role
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:registry-proxy@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/storage.objectViewer"

# Generate key
gcloud iam service-accounts keys create key.json \
  --iam-account=registry-proxy@PROJECT_ID.iam.gserviceaccount.com

# Use key.json content as password
```

### GitHub Container Registry (GHCR)

```json
{
  "name": "ghcr",
  "url": "https://ghcr.io",
  "username": "<GITHUB_USERNAME>",
  "password": "<GITHUB_PAT>",
  "mirror": false
}
```

**Pull Syntax:**
```bash
docker pull apps.afterdarksys.com:5005/proxy/ghcr/owner/repo:tag
```

**Setup:**
```bash
# Create GitHub Personal Access Token with:
# - read:packages scope
# Use as password in config
```

### AWS ECR

```json
{
  "name": "ecr",
  "url": "https://123456789012.dkr.ecr.us-east-1.amazonaws.com",
  "username": "AWS",
  "password": "<ECR_TOKEN>",
  "mirror": false
}
```

**Pull Syntax:**
```bash
docker pull apps.afterdarksys.com:5005/proxy/ecr/my-repo:tag
```

**Setup:**
```bash
# Get ECR login token (expires in 12 hours)
aws ecr get-login-password --region us-east-1

# Use as password (need to refresh every 12 hours)
```

### Quay.io

```json
{
  "name": "quay",
  "url": "https://quay.io",
  "username": "",
  "password": "",
  "mirror": false
}
```

**Pull Syntax:**
```bash
docker pull apps.afterdarksys.com:5005/proxy/quay/organization/repo:tag
```

---

## Configuration Reference

### Proxy Configuration

```json
{
  "enabled": true,              // Enable/disable proxy
  "cache_ttl_hours": 168,       // Keep cached images for 7 days
  "upstreams": [...],           // Upstream registries
  "remapping": {...}            // Path remapping rules
}
```

### Upstream Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique identifier (e.g., "dockerhub") |
| `url` | string | Yes | Registry base URL |
| `username` | string | No | Authentication username |
| `password` | string | No | Authentication password/token |
| `mirror` | boolean | No | Cache all pulls (true) or on-demand (false) |

### Remapping Rules

Simplify pull paths with remapping:

```json
{
  "remapping": {
    "ubuntu": "proxy/dockerhub/library/ubuntu",
    "nginx": "proxy/dockerhub/library/nginx",
    "myapp": "proxy/gcr/my-project/myapp"
  }
}
```

**Usage:**
```bash
# Instead of:
docker pull apps.afterdarksys.com:5005/proxy/dockerhub/library/ubuntu:latest

# Pull as:
docker pull apps.afterdarksys.com:5005/ubuntu:latest
```

---

## Cache Management

### Cache TTL

Images are cached based on `cache_ttl_hours`:

```json
{
  "cache_ttl_hours": 168  // 7 days
}
```

**Cleanup Policy:**
- Images older than TTL are eligible for deletion
- Garbage collection runs daily at 2 AM
- Actively pulled images reset their TTL

### Manual Cache Clear

**Clear specific image:**
```bash
curl -X DELETE https://apps.afterdarksys.com:5005/api/v2/cache/proxy/dockerhub/library/ubuntu:latest
```

**Clear all proxy cache:**
```bash
curl -X DELETE https://apps.afterdarksys.com:5005/api/v2/cache/proxy
```

**Prewarm cache:**
```bash
# Pull specific images before they're needed
curl -X POST https://apps.afterdarksys.com:5005/api/v2/cache/prewarm \
  -d '{"images": ["proxy/dockerhub/library/ubuntu:22.04", "proxy/gcr/google-samples/hello-app:1.0"]}'
```

---

## Deployment Scenarios

### Scenario 1: Docker Hub Rate Limit Bypass

**Problem:** Docker Hub limits anonymous pulls to 100/6 hours

**Solution:** Cache all Docker Hub pulls

```json
{
  "upstreams": [
    {
      "name": "dockerhub",
      "url": "https://registry-1.docker.io",
      "username": "<YOUR_DOCKERHUB_USERNAME>",
      "password": "<YOUR_DOCKERHUB_PAT>",
      "mirror": true
    }
  ]
}
```

**Configure Docker:**
```json
// /etc/docker/daemon.json
{
  "registry-mirrors": ["https://apps.afterdarksys.com:5005/proxy/dockerhub"]
}
```

**Result:** All pulls cached, bypass rate limits

### Scenario 2: Air-Gapped Deployment

**Problem:** Production cluster has no internet access

**Solution:** Prewarm cache with all required images

```bash
# In staging (with internet)
for image in ubuntu:22.04 nginx:alpine postgres:14; do
  docker pull apps.afterdarksys.com:5005/proxy/dockerhub/library/$image
done

# Sync registry storage to production
rsync -avz /var/lib/registry/ production-registry:/var/lib/registry/

# Production pulls from local cache (no internet needed)
docker pull production-registry:5005/proxy/dockerhub/library/ubuntu:22.04
```

### Scenario 3: Multi-Cloud Setup

**Problem:** Using images from GCR, ECR, and GHCR

**Solution:** Single proxy endpoint for all registries

```json
{
  "upstreams": [
    {"name": "gcr", "url": "https://gcr.io", ...},
    {"name": "ecr", "url": "https://123.dkr.ecr.us-east-1.amazonaws.com", ...},
    {"name": "ghcr", "url": "https://ghcr.io", ...}
  ]
}
```

**Usage:**
```bash
# All through same registry
docker pull apps.afterdarksys.com:5005/proxy/gcr/my-project/app:v1
docker pull apps.afterdarksys.com:5005/proxy/ecr/backend:latest
docker pull apps.afterdarksys.com:5005/proxy/ghcr/org/frontend:v2
```

### Scenario 4: CI/CD Acceleration

**Problem:** CI/CD pulls same base images repeatedly

**Solution:** Cache base images

```yaml
# .github/workflows/build.yml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Build
        run: |
          # Pull through registry proxy (cached after first run)
          docker build -t myapp \
            --build-arg BASE_IMAGE=apps.afterdarksys.com:5005/proxy/dockerhub/library/golang:1.21 .
```

**Result:**
- First CI run: pulls from Docker Hub (~2 minutes)
- Subsequent runs: cached (~5 seconds)

---

## Monitoring

### Cache Hit Rate

```bash
# Get cache statistics
curl https://apps.afterdarksys.com:5005/api/v2/cache/stats

{
  "total_requests": 1000,
  "cache_hits": 850,
  "cache_misses": 150,
  "hit_rate": "85%",
  "total_cached_size": "12.5 GB",
  "cached_images": 234
}
```

### Prometheus Metrics

```prometheus
# Cache hit rate
proxy_cache_hit_total / (proxy_cache_hit_total + proxy_cache_miss_total)

# Cached blob size
sum(proxy_cached_blob_bytes)

# Upstream fetch time
histogram_quantile(0.95, rate(proxy_upstream_fetch_duration_seconds_bucket[5m]))
```

### Grafana Dashboard

```
Total Cache Size: 12.5 GB
Hit Rate: 85%
Cached Images: 234

Top Cached Images:
  - ubuntu:22.04 (1.2 GB, 450 pulls)
  - nginx:alpine (45 MB, 320 pulls)
  - postgres:14 (380 MB, 180 pulls)

Upstream Latency:
  Docker Hub: 350ms avg
  GCR: 120ms avg
  GHCR: 280ms avg
```

---

## Security

### Authentication to Upstream

Always use credentials for private registries:

```json
{
  "upstreams": [
    {
      "name": "private-gcr",
      "url": "https://gcr.io",
      "username": "_json_key",
      "password": "<SERVICE_ACCOUNT_JSON>"
    }
  ]
}
```

### Image Scanning

Proxied images are automatically scanned:

```bash
# Scan happens automatically after cache
docker pull apps.afterdarksys.com:5005/proxy/dockerhub/library/ubuntu:latest

# Check scan results
curl https://apps.afterdarksys.com:5005/api/v2/scans/sha256:abc123/trivy
```

### Access Control

Restrict proxy access with RBAC:

```sql
-- Only allow specific users to pull from proxy
INSERT INTO access_policies (username, repository_pattern, permissions)
VALUES
  ('devteam', 'proxy/dockerhub/%', 'pull'),
  ('admin', 'proxy/%', 'pull,push,delete');
```

---

## Troubleshooting

### Issue: 401 Unauthorized from Upstream

**Error:**
```
failed to fetch manifest: upstream returned status 401
```

**Fix:**
1. Verify credentials in `proxy.json`
2. Check token expiration (ECR tokens expire in 12h)
3. Test authentication manually:
   ```bash
   curl -u username:password https://registry-1.docker.io/v2/
   ```

### Issue: Slow First Pull

**Error:**
```
Pulling image takes 5 minutes
```

**Explanation:** First pull downloads from upstream (expected)

**Fix:**
1. Prewarm cache before deployment:
   ```bash
   ./scripts/prewarm-cache.sh ubuntu:22.04 nginx:alpine
   ```

2. Use mirror mode for frequently pulled registries:
   ```json
   {"mirror": true}
   ```

### Issue: Cache Not Updating

**Error:**
```
Image updated upstream but still getting old version
```

**Fix:**
1. Clear specific image cache:
   ```bash
   curl -X DELETE https://apps.afterdarksys.com:5005/api/v2/cache/proxy/dockerhub/library/ubuntu:latest
   ```

2. Reduce cache TTL:
   ```json
   {"cache_ttl_hours": 24}  // 1 day instead of 7
   ```

3. Force pull with digest:
   ```bash
   docker pull apps.afterdarksys.com:5005/proxy/dockerhub/library/ubuntu@sha256:abc123
   ```

### Issue: Disk Space Full

**Error:**
```
failed to write blob: no space left on device
```

**Fix:**
1. Run garbage collection:
   ```bash
   curl -X POST https://apps.afterdarksys.com:5005/api/v2/gc/run
   ```

2. Reduce cache TTL
3. Monitor disk usage:
   ```bash
   df -h /var/lib/registry
   ```

---

## Performance Optimization

### 1. Dedicated Cache Disk

Mount fast SSD for cache storage:

```bash
# Mount NVMe SSD for registry blobs
mount /dev/nvme0n1 /var/lib/registry/blobs
```

### 2. CDN Integration

Serve blobs through CDN:

```nginx
# nginx.conf
location ~ ^/v2/.*/blobs/ {
    proxy_pass https://apps.afterdarksys.com:5005;
    proxy_cache registry_cache;
    proxy_cache_valid 200 7d;
}
```

### 3. Parallel Layer Downloads

Configure Docker to download layers in parallel:

```json
// /etc/docker/daemon.json
{
  "max-concurrent-downloads": 10
}
```

### 4. Compression

Enable blob compression:

```json
{
  "compression": {
    "enabled": true,
    "level": 6
  }
}
```

---

## API Reference

### Check Cache Status

```bash
GET /api/v2/cache/proxy/{upstream}/{repo}:{tag}/status

Response:
{
  "cached": true,
  "cached_at": "2026-03-12T10:00:00Z",
  "size_bytes": 1234567,
  "expires_at": "2026-03-19T10:00:00Z"
}
```

### Trigger Prewarm

```bash
POST /api/v2/cache/prewarm

{
  "images": [
    "proxy/dockerhub/library/ubuntu:22.04",
    "proxy/gcr/google-samples/hello-app:1.0"
  ]
}
```

### Clear Cache

```bash
DELETE /api/v2/cache/proxy/{upstream}/{repo}:{tag}
```

---

## Best Practices

### 1. Use Authenticated Pulls

Always configure credentials for upstream registries to avoid rate limits.

### 2. Monitor Cache Hit Rate

Target 80%+ hit rate. If lower, increase cache TTL or prewarm common images.

### 3. Regular Cleanup

Run garbage collection weekly:

```bash
0 2 * * 0 curl -X POST https://apps.afterdarksys.com:5005/api/v2/gc/run
```

### 4. Separate Proxy Namespace

Keep proxied images separate from your own:

```
apps.afterdarksys.com:5005/myapp/frontend:v1      # Your images
apps.afterdarksys.com:5005/proxy/dockerhub/...    # Proxied images
```

### 5. Mirror Mission-Critical Registries

Use `"mirror": true` for critical upstreams (Docker Hub):

```json
{"name": "dockerhub", "mirror": true}
```

---

## Comparison with Other Solutions

| Solution | Caching | Multi-Registry | Transparent | Auth |
|----------|---------|----------------|-------------|------|
| **ADS Registry Proxy** | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Yes |
| **Docker Registry Proxy** | ✅ Yes | ❌ Single | ⚠️ Requires config | ⚠️ Limited |
| **Artifactory** | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Yes ($$$$) |
| **Harbor Proxy** | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Yes |

---

**Questions?** Check [GitHub Discussions](https://github.com/ryan/ads-registry/discussions) or file an [issue](https://github.com/ryan/ads-registry/issues).
