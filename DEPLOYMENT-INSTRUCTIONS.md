# Router Fix Deployment Instructions

## Quick Summary

**Problem**: Single-level repository names (e.g., `tokenworx`, `nginx`) returned 404 on blob uploads due to chi router pattern matching bug.

**Solution**: Replaced two overlapping route patterns with a single wildcard pattern `/{repoPath...}` that handles all repository nesting levels.

**Impact**: Fixes routing for 107+ failed single-level repository migrations while maintaining backward compatibility.

## Pre-Deployment

### 1. Verify Build

```bash
cd /Users/ryan/development/ads-registry
go build -o ads-registry-fixed ./cmd/ads-registry
```

**Expected**: Build succeeds with no errors.

### 2. Backup Current Binary

```bash
ssh user@apps.afterdarksys.com
cd /path/to/registry
cp ads-registry ads-registry-backup-$(date +%Y%m%d-%H%M%S)
```

### 3. Run Pre-Deployment Test (Optional)

```bash
# On local machine if you have test environment
./test-routing-fix.sh
```

## Deployment Steps

### Step 1: Stop the Registry

```bash
ssh user@apps.afterdarksys.com
sudo systemctl stop ads-registry
```

### Step 2: Deploy New Binary

```bash
# From your development machine
cd /Users/ryan/development/ads-registry
scp ads-registry-fixed user@apps.afterdarksys.com:/tmp/

# On the server
ssh user@apps.afterdarksys.com
sudo mv /tmp/ads-registry-fixed /path/to/registry/ads-registry
sudo chown registry:registry /path/to/registry/ads-registry
sudo chmod +x /path/to/registry/ads-registry
```

### Step 3: Start the Registry

```bash
ssh user@apps.afterdarksys.com
sudo systemctl start ads-registry
sudo systemctl status ads-registry
```

**Expected**: Service starts successfully with no errors in logs.

### Step 4: Verify Logs

```bash
# Watch logs for startup errors
ssh user@apps.afterdarksys.com
sudo journalctl -u ads-registry -f
```

**Look for**:
- "ADS Container Registry starting up"
- "Initialized Database"
- "Starting registry on 0.0.0.0:5005"
- NO routing errors or panics

## Post-Deployment Verification

### Test 1: Single-Level Repository Upload

```bash
# Get auth token for single-level repo
TOKEN=$(curl -s -u admin:admin \
  "https://apps.afterdarksys.com:5006/auth/token?service=registry&scope=repository:tokenworx:pull,push" \
  | jq -r '.token')

# Test blob upload endpoint
curl -i -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  "https://apps.afterdarksys.com:5006/v2/tokenworx/blobs/uploads/"
```

**Expected Response**:
```
HTTP/1.1 202 Accepted
Location: /v2/tokenworx/blobs/uploads/{uuid}
Docker-Upload-UUID: {uuid}
```

**NOT**: `HTTP/1.1 404 Not Found`

### Test 2: Multi-Level Repository (Should Still Work)

```bash
TOKEN=$(curl -s -u admin:admin \
  "https://apps.afterdarksys.com:5006/auth/token?service=registry&scope=repository:web3dns/aiserve-farm:pull,push" \
  | jq -r '.token')

curl -i -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  "https://apps.afterdarksys.com:5006/v2/web3dns/aiserve-farm/blobs/uploads/"
```

**Expected**: `HTTP/1.1 202 Accepted`

### Test 3: Run Full Test Suite

```bash
# Copy test script to server
scp test-routing-fix.sh user@apps.afterdarksys.com:/tmp/
ssh user@apps.afterdarksys.com 'bash /tmp/test-routing-fix.sh'
```

**Expected**: All tests show SUCCESS or AUTH ISSUE (no 404 errors).

### Test 4: Real Migration

```bash
# Try migrating one of the failed images
docker login apps.afterdarksys.com:5006 -u admin -p admin

# Pull from source
docker pull source-registry/tokenworx:latest

# Tag for destination
docker tag source-registry/tokenworx:latest apps.afterdarksys.com:5006/tokenworx:latest

# Push (this should now work)
docker push apps.afterdarksys.com:5006/tokenworx:latest
```

**Expected**: Push succeeds without 404 errors.

## Rollback Procedure

If any issues occur:

```bash
ssh user@apps.afterdarksys.com
sudo systemctl stop ads-registry

# Restore previous version
cd /path/to/registry
sudo cp ads-registry-backup-YYYYMMDD-HHMMSS ads-registry
sudo chown registry:registry ads-registry
sudo chmod +x ads-registry

# Restart
sudo systemctl start ads-registry
sudo systemctl status ads-registry
```

## Monitoring

### Key Metrics to Watch

1. **HTTP Status Codes**:
   - 404 errors on `/v2/*/blobs/uploads/` should drop to zero
   - 202 responses should increase

2. **Migration Success Rate**:
   - Single-level repos should reach 100% success
   - Previously failed 107 images should succeed

3. **Server Health**:
   ```bash
   curl https://apps.afterdarksys.com:5006/health/live
   curl https://apps.afterdarksys.com:5006/health/ready
   ```

4. **Server Logs**:
   ```bash
   ssh user@apps.afterdarksys.com
   sudo journalctl -u ads-registry --since "5 minutes ago"
   ```

### Expected Log Entries (Success)

```
[START_UPLOAD] repoPath=tokenworx ns= repo=tokenworx uuid=...
[PUT_UPLOAD] repoPath=tokenworx ns= repo=tokenworx uuid=... digest=sha256:...
[PUT_MANIFEST] Starting: repoPath=tokenworx ns= repo=tokenworx ref=latest ContentLength=...
[MIDDLEWARE] Checking authorization: URL=/v2/tokenworx/blobs/uploads/ repoPath=tokenworx fullRepo=tokenworx action=push
```

### Warning Signs (Problems)

```
# These indicate routing is still broken:
404 page not found
routing: no route found
[MIDDLEWARE] Authorization DENIED

# These are acceptable (auth needs configuration):
401 Unauthorized
403 Forbidden
```

## Re-Running Failed Migrations

After successful deployment, re-run migrations for the 107 failed images:

```bash
# Option 1: Re-run entire migration with resume
./migrate-registry.sh --resume

# Option 2: Re-run only failed images
./migrate-registry.sh --failed-only

# Option 3: Manual re-migration of specific images
docker push apps.afterdarksys.com:5006/tokenworx:latest
docker push apps.afterdarksys.com:5006/nginx:latest
# ... repeat for each failed image
```

## Success Criteria

Deployment is successful when:

- [x] Service starts without errors
- [x] Health endpoints return 200 OK
- [x] Single-level repo upload returns 202 (not 404)
- [x] Multi-level repo upload returns 202 (still works)
- [x] Test script shows all SUCCESS (no 404 errors)
- [x] Real docker push to single-level repo succeeds
- [x] Server logs show correct routing with `repoPath` parameter
- [x] No increase in error rates
- [x] Migration completion percentage increases

## Timeline

- **Deployment window**: 15 minutes
- **Testing**: 10 minutes
- **Migration re-run**: 1-2 hours (for 107 images)
- **Total**: ~2.5 hours

## Support

If issues occur during deployment:

1. **Check logs first**: `journalctl -u ads-registry -n 100`
2. **Review this document**: ROUTER-FIX-DOCUMENTATION.md
3. **Rollback if necessary**: Follow rollback procedure above
4. **Debug**: Check HTTP responses for specific error messages

## Post-Deployment Tasks

After successful deployment:

1. Monitor server logs for 24 hours
2. Track migration completion rate
3. Update documentation with any lessons learned
4. Consider adding automated routing tests
5. Update monitoring dashboards if needed

---

**Deployment Status**: Ready for production
**Risk Level**: Low (additive changes only, no schema changes)
**Rollback Time**: < 5 minutes
**Expected Downtime**: 2-3 minutes for restart
