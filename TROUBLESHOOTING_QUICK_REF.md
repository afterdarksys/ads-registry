# Container Registry Troubleshooting Quick Reference

## "short copy" Errors

### Symptom
```
Error: failed to do request: Put "https://apps.afterdarksys.com/v2/test/alpine/manifests/latest":
short copy: wrote 1 of 8958
```

### Root Cause
Docker 29.2.0 client bug - reads response too quickly and closes connection

### Quick Fix
```bash
# Check if compatibility workarounds are active
docker logs ads-registry | grep -i compat

# Should see:
# [COMPAT] Compatibility system enabled
# [COMPAT] Client detected: Docker/29.2.0
# [COMPAT] Activating workarounds for Docker/29.2.0: [docker_29_manifest_fix]

# If NOT seen, compatibility is not configured correctly
# Verify config.json has:
{
  "compatibility": {
    "enabled": true,
    "docker_client_workarounds": {
      "enable_docker_29_manifest_fix": true,
      "extra_flushes": 5
    }
  }
}
```

### Workaround (If Fix Doesn't Work)
```bash
# Downgrade Docker client temporarily
# macOS:
brew install --cask docker@28

# Or use podman instead:
brew install podman
podman push apps.afterdarksys.com/test/alpine:latest
```

---

## Timeout Errors

### Symptom
```
Error: context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

### Root Cause
Registry timeout (5min) too short for large images on slow network

### Quick Fix
```bash
# Check current timeout settings
grep -A5 '"server"' config.json | grep timeout

# Should see 1800000000000 (30 minutes)
# If showing 300000000000 (5 minutes), update config.json:
{
  "server": {
    "read_timeout": 1800000000000,
    "write_timeout": 1800000000000
  }
}

# Restart registry
docker compose down && docker compose up -d
```

### Calculate Required Timeout
```bash
# Formula: timeout_seconds = (layer_size_MB / network_speed_Mbps) * 8 * 1.5

# Example: 1GB layer on 2Mbps WiFi
# (1024 / 2) * 8 * 1.5 = 6144 seconds = ~102 minutes

# For WiFi, use 30min minimum (1800 seconds)
```

---

## High Disk I/O

### Symptom
```bash
# Check with iostat
iostat -x 1

# If seeing >1000 IOPS during push with 100% utilization
Device   r/s   w/s   %util
sda      10   4200   99.8   # BAD - unbuffered I/O
```

### Root Cause
Missing buffered I/O wrapper in storage layer

### Quick Fix
```bash
# Verify bufferedFileWriter is implemented
grep -n "bufferedFileWriter" internal/storage/local/local.go

# Should return multiple matches
# If NOT found, buffered I/O not implemented

# Rebuild registry
go build -o ads-registry ./cmd/ads-registry/
docker build -t ads-registry:latest .
docker compose up -d
```

---

## Manifest Push Succeeds, But Image Won't Pull

### Symptom
```bash
docker pull apps.afterdarksys.com/test/alpine:latest
# Error: manifest unknown
```

### Root Cause
Digest mismatch - manifest stored with wrong digest

### Diagnosis
```bash
# Check database
sqlite3 data/registry.db "SELECT reference, digest FROM manifests WHERE repository='test/alpine';"

# Verify blob exists
ls -lh data/blobs/sha256/abc.../data

# Check manifest content
curl -H "Authorization: Bearer TOKEN" \
  https://apps.afterdarksys.com/v2/test/alpine/manifests/latest
```

### Fix
```bash
# Re-push with digest verification
docker push apps.afterdarksys.com/test/alpine:latest

# If still fails, check registry logs
docker logs ads-registry | grep DIGEST_INVALID
```

---

## Database Lock Errors

### Symptom
```
database is locked (5000ms)
```

### Root Cause
SQLite WAL mode not enabled, or too many concurrent writers

### Quick Fix
```bash
# Check WAL mode
sqlite3 data/registry.db "PRAGMA journal_mode;"
# Should return: wal

# If not, enable WAL
sqlite3 data/registry.db "PRAGMA journal_mode=WAL;"

# Verify DSN in config.json
grep dsn config.json
# Should have: _journal_mode=WAL&_busy_timeout=5000

# Check concurrent connections
grep max_open_conns config.json
# Should be: 25 (SQLite limit)
# If higher, reduce to 25
```

### When to Migrate to PostgreSQL
- Database locks occur frequently (>10/hour)
- More than 50 concurrent clients
- Storage exceeds 5TB

---

## Network Connectivity Issues

### Test Registry Reachability
```bash
# 1. Basic connectivity
curl -v https://apps.afterdarksys.com/v2/

# Expected: 401 Unauthorized (correct - needs auth)
# Bad: Connection refused, timeout, or SSL errors

# 2. Check DNS
nslookup apps.afterdarksys.com
# Should resolve to correct IP

# 3. Check firewall
telnet apps.afterdarksys.com 443
# Should connect (Ctrl+C to exit)

# 4. Test with authentication
TOKEN=$(curl -s "https://apps.afterdarksys.com/auth/token?service=registry&scope=repository:test/alpine:pull" | jq -r .token)
curl -H "Authorization: Bearer $TOKEN" https://apps.afterdarksys.com/v2/test/alpine/manifests/latest
```

### Traefik Issues
```bash
# Check Traefik routing
docker logs traefik | grep registry

# Verify backend health
curl http://localhost:5005/health/ready
# Should return: {"status":"ok"}

# Check Traefik config
cat /etc/traefik/dynamic/registry.yml
```

---

## Performance Optimization Checklist

### Registry Server
- [ ] Buffered I/O enabled (internal/storage/local/local.go has bufferedFileWriter)
- [ ] TCP_NODELAY enabled (serve.go line 555)
- [ ] TCP buffers set to 1MB (serve.go line 557-558)
- [ ] Timeouts set to 30min for slow networks (config.json server.read_timeout)
- [ ] SQLite WAL mode enabled (config.json database.dsn)
- [ ] Compatibility workarounds enabled (config.json compatibility.enabled=true)

### Client Side
- [ ] Docker client version checked (avoid 29.x if possible)
- [ ] Network stable (ping apps.afterdarksys.com - <100ms latency, <5% packet loss)
- [ ] DNS resolving correctly (nslookup apps.afterdarksys.com)
- [ ] No proxy interfering (check HTTP_PROXY, HTTPS_PROXY env vars)

### Traefik (If Used)
- [ ] Backend timeout ≥30min (traefik.yml http.middlewares.registry-timeout.timeout.read)
- [ ] Buffering enabled for large requests (traefik.yml http.middlewares.buffering)
- [ ] Health checks passing (docker logs traefik | grep "registry.*healthy")

---

## Emergency Commands

### Force Clean Restart
```bash
docker compose down
docker system prune -af --volumes  # WARNING: Deletes all unused data
docker build -t ads-registry:latest .
docker compose up -d
```

### Reset Registry Database (DESTRUCTIVE)
```bash
# Backup first
cp data/registry.db data/registry.db.backup.$(date +%Y%m%d_%H%M%S)

# Reset
rm data/registry.db
docker compose up -d

# Registry will recreate empty database on startup
```

### Export Metrics
```bash
# Prometheus metrics
curl http://apps.afterdarksys.com:5005/metrics > registry-metrics.txt

# Compatibility metrics
grep ads_registry_compat registry-metrics.txt

# Request duration histogram
grep http_request_duration_seconds registry-metrics.txt
```

---

## Log Analysis

### Find Failed Pushes
```bash
docker logs ads-registry 2>&1 | grep -i "error\|fail" | grep -i push
```

### Find Timeout Issues
```bash
docker logs ads-registry 2>&1 | grep -i "timeout\|deadline"
```

### Find Compatibility Workarounds
```bash
docker logs ads-registry 2>&1 | grep "\[COMPAT\]"
```

### Find Disk I/O Issues
```bash
# On registry server
iostat -x 5 10 > iostat-output.txt
# Look for %util near 100% during pushes

# Check kernel logs for filesystem errors
dmesg | grep -i "error\|fail" | grep -v "audit"
```

---

## Contact & Escalation

### Before Escalating
1. Collect logs: `docker logs ads-registry > registry.log`
2. Collect metrics: `curl http://localhost:5005/metrics > metrics.txt`
3. Document error: Screenshot or copy full error message
4. Test with simple image: `docker push apps.afterdarksys.com/test/alpine:latest`

### Escalation Information
- **Registry Version:** Check `git log -1` in registry directory
- **Docker Client Version:** `docker version`
- **Network Info:** `speedtest-cli` or `ping apps.afterdarksys.com`
- **OS Info:** `uname -a` (server) and client OS
- **Storage Info:** `df -h data/` (registry server)
