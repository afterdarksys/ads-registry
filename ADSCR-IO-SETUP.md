# 🚀 adscr.io Setup Guide

## Why adscr.io is PERFECT

- **Short & Memorable**: 8 characters vs 30+ for subdomains
- **Professional**: Clean, modern SaaS branding
- **Docker CLI Ready**: `docker pull afterdark.adscr.io/myapp:latest`
- **Customer Friendly**: Easy to share and remember

---

## Your New Multi-Tenant URLs

| Tenant | Old URL | New URL ⭐ |
|--------|---------|-----------|
| Default | `registry.afterdarksys.com` | `adscr.io` |
| AfterDark | `afterdark.registry.afterdarksys.com` | `afterdark.adscr.io` |
| Customer A | `acme.registry.afterdarksys.com` | `acme.adscr.io` |

**Example Docker commands:**
```bash
# Pull from your tenant
docker pull afterdark.adscr.io/webapp:v1.0

# Pull from customer tenant
docker pull acme.adscr.io/api:latest

# Public registry (root domain)
docker pull adscr.io/public/nginx:alpine
```

---

## 🎯 Quick DNS Setup (5 Minutes)

### Option A: Oracle Cloud Console

1. **Login to Oracle Cloud**
   - Go to: https://cloud.oracle.com
   - Navigate to: **Networking** → **DNS Management** → **Zones**

2. **Select adscr.io zone**
   - Click on your `adscr.io` zone

3. **Add Wildcard Record**
   ```
   Type: A
   Name: *
   Address: 108.165.123.229
   TTL: 600
   ```
   Click **Add Record**

4. **Add Root Domain Record**
   ```
   Type: A
   Name: @
   Address: 108.165.123.229
   TTL: 600
   ```
   Click **Add Record**

5. **Publish Changes**
   - Click **Publish Changes** button

### Option B: Oracle OCI CLI (Faster!)

```bash
# Add wildcard record
oci dns record domain patch \
  --zone-name-or-id adscr.io \
  --domain "*.adscr.io" \
  --scope GLOBAL \
  --items '[{"domain":"*.adscr.io","rdata":"108.165.123.229","rtype":"A","ttl":600}]'

# Add root domain record
oci dns record domain patch \
  --zone-name-or-id adscr.io \
  --domain "adscr.io" \
  --scope GLOBAL \
  --items '[{"domain":"adscr.io","rdata":"108.165.123.229","rtype":"A","ttl":600}]'
```

### Step 4: Wait & Test (5-10 minutes)
```bash
# Test DNS resolution
dig adscr.io
dig afterdark.adscr.io
dig test.adscr.io

# All should return: 108.165.123.229
```

---

## 🔧 Update Server Configuration

After DNS propagates, update the registry:

```bash
# SSH to server
ssh root@apps.afterdarksys.com

# Edit systemd service
sudo nano /etc/systemd/system/ads-registry.service
```

Add these lines under `[Service]`:
```ini
Environment="REGISTRY_BASE_DOMAIN=adscr.io"
Environment="REGISTRY_BASE_URL=https://adscr.io"
```

Restart the service:
```bash
sudo systemctl daemon-reload
sudo systemctl restart ads-registry.service
```

---

## ✅ Test Multi-Tenant Access

```bash
# Test root domain
curl http://adscr.io:5005/v2/
# Expected: 401 Unauthorized (auth required - good!)

# Test afterdark tenant
curl http://afterdark.adscr.io:5005/v2/
# Expected: 401 Unauthorized

# Watch tenant resolution in logs
ssh root@apps.afterdarksys.com
journalctl -u ads-registry.service -f
```

Make a request and look for:
```
Multi-tenancy enabled: tenant resolved from subdomain
Tenant: afterdark (ID: 2) resolved from subdomain
```

---

## 🔒 Optional: SSL/TLS with Let's Encrypt

Get wildcard certificate for all tenants:

```bash
# Install certbot
apt-get install certbot python3-certbot-dns-godaddy

# Create GoDaddy API credentials file
mkdir -p /root/.secrets
nano /root/.secrets/godaddy.ini
```

Add to `godaddy.ini`:
```ini
dns_godaddy_key = YOUR_GODADDY_API_KEY
dns_godaddy_secret = YOUR_GODADDY_API_SECRET
```

Get wildcard certificate:
```bash
chmod 600 /root/.secrets/godaddy.ini

certbot certonly --dns-godaddy \
  --dns-godaddy-credentials /root/.secrets/godaddy.ini \
  -d "adscr.io" \
  -d "*.adscr.io"
```

Update config:
```bash
nano /root/ads-registry/config.production.json
```

```json
{
  "tls": {
    "enabled": true,
    "cert_file": "/etc/letsencrypt/live/adscr.io/fullchain.pem",
    "key_file": "/etc/letsencrypt/live/adscr.io/privkey.pem"
  }
}
```

Restart:
```bash
systemctl restart ads-registry.service
```

Now test with HTTPS:
```bash
curl https://afterdark.adscr.io/v2/
```

---

## 📊 Customer Onboarding Example

When you onboard a new customer:

```bash
# 1. Create tenant in database
docker exec -i letsgoout-postgres psql -U ads_registry -d ads_registry <<SQL
INSERT INTO tenants (slug, name, schema_name, status, contact_email)
VALUES ('acme', 'Acme Corporation', 'tenant_acme', 'active', 'admin@acme.com')
RETURNING id;

SELECT provision_tenant_schema('tenant_acme');
SQL

# 2. That's it! Their registry is live at:
# acme.adscr.io
```

No DNS changes needed - the wildcard handles all subdomains!

---

## 🎉 What You Get

✅ **Instant tenant provisioning** - No DNS changes per customer
✅ **Clean branding** - `customer.adscr.io` vs `customer.registry.afterdarksys.com`
✅ **Docker-native** - Works seamlessly with `docker pull/push`
✅ **SSL ready** - Single wildcard cert covers all tenants
✅ **Professional** - Looks like a real SaaS product

---

## 🚀 Status

- ✅ Multi-tenancy code deployed
- ✅ Database schema ready
- ✅ 2 tenants provisioned (default, afterdark)
- ⏳ **DNS setup pending** (you're doing this now!)
- ⏳ SSL certificate (optional, do after DNS)

---

**Domain registered**: adscr.io
**Server**: apps.afterdarksys.com (108.165.123.229)
**Status**: Ready to go live! 🔥
