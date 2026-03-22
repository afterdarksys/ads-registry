# ADS Registry - Production Deployment Status

**Last Updated:** 2026-03-22
**Deployed By:** Claude Code

## ✅ Successfully Deployed

### Application Status
- **Service:** Running and healthy
- **URL:** https://registry.afterdarksys.com
- **Health:** https://registry.afterdarksys.com/health/ready
- **API:** https://registry.afterdarksys.com/v2/

### TCP Tuning Applied

**OS-Level Settings:**
- ✅ TCP buffer sizes: 128MB (was ~4MB)
- ✅ Connection queue: 4096 (was 128)
- ✅ Slow start after idle: Disabled
- ✅ Keepalive: Optimized (60s detection)
- ✅ Port range: 10000-65535
- ✅ File descriptors: 65536

**Application Settings:**
- ✅ Read timeout: 600s (10 minutes)
- ✅ Write timeout: 600s (10 minutes)
- ✅ Idle timeout: 120s (2 minutes)
- ✅ Read header timeout: 30s
- ✅ Max header bytes: 50MB

### Files Deployed

**Binary:**
- `/opt/ads-registry/ads-registry` (28MB)
- Built with: CGO_ENABLED=1 (for SQLite support)
- Includes TCP tuning improvements

**Configuration:**
- `/opt/ads-registry/config.json`
- Using PostgreSQL on localhost:5434
- OIDC: Disabled (awaiting Authentik configuration)

**Scripts:**
- `/opt/ads-registry/tune-tcp.sh` (TCP tuning automation)

### Backups Created

- `ads-registry.backup-20260321-*` (previous binary)
- `config.json.backup-20260321-*` (previous config)
- `/etc/sysctl.d/99-ads-registry-backup-*.conf` (TCP settings backup)

## 🔐 OAuth/OIDC Status

**Status:** ⏳ Pending Configuration

**Next Steps:**

1. **Configure Authentik OAuth Provider:**
   - See `docs/AUTHENTIK_SETUP.md` for manual setup guide
   - OR run `scripts/configure-authentik-oauth.py` with API token

2. **Required Settings:**
   ```json
   "oidc": {
     "enabled": true,
     "issuer": "https://auth.afterdarksys.com/application/o/ads-registry/",
     "client_id": "ads-registry",
     "client_secret": "ca2ae983e7d7d4ad45e619a0da7f2079b7ec4e5510e370b0bddc45b6c6758d14",
     "redirect_url": "https://registry.afterdarksys.com/oauth2/callback",
     "scopes": ["openid", "profile", "email", "groups"]
   }
   ```

3. **After Authentik is configured:**
   ```bash
   # Update config with oidc.enabled = true
   scp config.production.json root@apps.afterdarksys.com:/opt/ads-registry/config.json

   # Restart registry
   ssh root@apps.afterdarksys.com "systemctl restart ads-registry"
   ```

## 📊 Performance Improvements

### Before TCP Tuning
- Max upload timeout: 5 minutes
- TCP buffer: 4MB
- Connection queue: 128
- Slow start: Enabled (bandwidth drops on idle)

### After TCP Tuning
- Max upload timeout: 10 minutes (configurable to 60m)
- TCP buffer: 128MB
- Connection queue: 4096
- Slow start: Disabled (maintains bandwidth)

**Expected Benefits:**
- ✅ Handles multi-GB images without timeout
- ✅ Better performance on high-latency networks
- ✅ No connection drops during large transfers
- ✅ Higher concurrent connection capacity

## 🧪 Testing

### Quick Health Check
```bash
# Health endpoint
curl https://registry.afterdarksys.com/health/ready

# API v2 endpoint
curl -I https://registry.afterdarksys.com/v2/
```

### Push Test Image
```bash
# Login (with personal access token)
docker login registry.afterdarksys.com

# Tag and push
docker tag nginx:latest registry.afterdarksys.com/test/nginx:latest
docker push registry.afterdarksys.com/test/nginx:latest
```

### Verify TCP Settings
```bash
ssh root@apps.afterdarksys.com "sysctl net.core.rmem_max net.core.wmem_max"
# Should show: net.core.rmem_max = 134217728
```

## 📁 Documentation

- `docs/TCP_TUNING.md` - Complete TCP tuning guide
- `docs/AUTHENTIK_SETUP.md` - Authentik OAuth setup guide
- `scripts/tune-tcp.sh` - Automated TCP tuning script
- `scripts/configure-authentik-oauth.py` - OAuth automation script

## 🔧 Maintenance

### View Logs
```bash
ssh root@apps.afterdarksys.com "journalctl -u ads-registry -f"
```

### Restart Service
```bash
ssh root@apps.afterdarksys.com "systemctl restart ads-registry"
```

### Rollback if Needed
```bash
# Restore previous binary
ssh root@apps.afterdarksys.com "cd /opt/ads-registry && \
  cp ads-registry.backup-20260321-* ads-registry && \
  systemctl restart ads-registry"

# Restore TCP settings
ssh root@apps.afterdarksys.com "sysctl -p /etc/sysctl.d/99-ads-registry-backup-*.conf"
```

## 🚀 Next Steps

1. ⏳ **Configure Authentik OAuth** (see docs/AUTHENTIK_SETUP.md)
2. ⏳ **Enable OIDC in config.json**
3. ⏳ **Test OAuth login flow**
4. ⏳ **Push a large test image** (multi-GB) to verify TCP tuning
5. ⏳ **Monitor performance** under load

## ✅ Deployment Checklist

- [x] Build Linux binary with TCP tuning code
- [x] Upload binary to production server
- [x] Apply OS-level TCP tuning
- [x] Update application configuration
- [x] Backup existing files
- [x] Deploy new binary
- [x] Restart service
- [x] Verify health endpoints
- [x] Verify TCP settings applied
- [ ] Configure Authentik OAuth provider
- [ ] Enable OIDC in registry
- [ ] Test OAuth login
- [ ] Test large image upload

---

**Deployment successful! 🎉**

Registry is running with optimized TCP settings for handling large container images.
OAuth/OIDC authentication pending Authentik configuration.
