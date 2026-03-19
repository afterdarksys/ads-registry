# DNS Setup for Multi-Tenant Registry

## Your Server Information

- **Server IP**: `108.165.123.229`
- **Primary Domain**: `adscr.io` ⭐ (NEW!)
- **Legacy Domain**: `afterdarksys.com`
- **DNS Provider**: Oracle Cloud
- **Current Working**: `apps.afterdarksys.com` → `108.165.123.229`

---

## DNS Records to Add

Log into your **Oracle Cloud DNS Management** for **adscr.io** and add these records:

### Option 1: Wildcard Subdomain (Recommended)

Add **TWO** records:

```
Type: A
Name: @
Value: 108.165.123.229
TTL: 600 (10 minutes)

Type: A
Name: *
Value: 108.165.123.229
TTL: 600 (10 minutes)
```

**This enables:**
- `adscr.io` → Default tenant / Landing page
- `afterdark.adscr.io` → Your "afterdark" tenant
- `acme.adscr.io` → Future "acme" tenant
- `*.adscr.io` → Any tenant!

### Option 2: Individual Subdomains (If wildcard not allowed)

Add multiple records:

```
Type: A
Name: afterdark
Value: 108.165.123.229
TTL: 600

Type: A
Name: @
Value: 108.165.123.229
TTL: 600
```

---

## Step-by-Step Oracle Cloud Instructions

### Method 1: Oracle Cloud Console (Web UI)

1. **Login to Oracle Cloud**: https://cloud.oracle.com

2. **Navigate to DNS**:
   - Click hamburger menu (≡)
   - Go to **Networking** → **DNS Management** → **Zones**

3. **Select your zone**:
   - Click on `adscr.io`

4. **Add wildcard record**:
   - Click **Add Record** button
   - **Type**: Select "A"
   - **Name**: Enter `*`
   - **Address**: Enter `108.165.123.229`
   - **TTL**: Enter `600`
   - Click **Add**

5. **Add root domain record**:
   - Click **Add Record** button again
   - **Type**: Select "A"
   - **Name**: Leave empty or enter `@`
   - **Address**: Enter `108.165.123.229`
   - **TTL**: Enter `600`
   - Click **Add**

6. **Publish changes**:
   - Click **Publish Changes** button at top

7. **Wait 5-10 minutes** for DNS propagation

### Method 2: OCI CLI (Command Line - Faster!)

```bash
# List existing records first
oci dns record domain get \
  --zone-name-or-id adscr.io \
  --domain adscr.io

# Add wildcard A record
oci dns record domain patch \
  --zone-name-or-id adscr.io \
  --domain "*.adscr.io" \
  --scope GLOBAL \
  --items '[{"domain":"*.adscr.io","rdata":"108.165.123.229","rtype":"A","ttl":600}]'

# Add root domain A record
oci dns record domain patch \
  --zone-name-or-id adscr.io \
  --domain "adscr.io" \
  --scope GLOBAL \
  --items '[{"domain":"adscr.io","rdata":"108.165.123.229","rtype":"A","ttl":600}]'
```

---

## Test DNS Propagation

After adding the DNS record, test it:

```bash
# Test root domain
dig adscr.io

# Test wildcard subdomain
dig afterdark.adscr.io

# Should return:
# adscr.io. 600 IN A 108.165.123.229
# afterdark.adscr.io. 600 IN A 108.165.123.229

# Or use online tool:
# https://dnschecker.org/#A/adscr.io
# https://dnschecker.org/#A/afterdark.adscr.io
```

---

## Update Registry Configuration

After DNS is working, update your registry base domain:

```bash
# SSH to server
ssh root@apps.afterdarksys.com

# Update systemd service with new domain
sudo nano /etc/systemd/system/ads-registry.service

# Add under [Service]:
Environment="REGISTRY_BASE_DOMAIN=adscr.io"
Environment="REGISTRY_BASE_URL=https://adscr.io"

# Reload and restart
sudo systemctl daemon-reload
sudo systemctl restart ads-registry.service
```

---

## Test Multi-Tenant Access

Once DNS propagates:

```bash
# Test afterdark tenant
curl http://afterdark.adscr.io:5005/v2/
# Should return: 401 Unauthorized (auth required - good!)

# Test root domain (default tenant)
curl http://adscr.io:5005/v2/
# Should return: 401 Unauthorized

# Check tenant resolution in logs
ssh root@apps.afterdarksys.com
journalctl -u ads-registry.service -f
# Make a request and watch for tenant resolution logs
```

---

## SSL/TLS Setup (Optional but Recommended)

For HTTPS on subdomains, you have two options:

### Option A: Wildcard Certificate (Recommended)

Use Let's Encrypt with DNS challenge:

```bash
# Install certbot with DNS plugin
apt-get install certbot python3-certbot-dns-godaddy

# Get wildcard cert for adscr.io
certbot certonly --dns-godaddy \
  --dns-godaddy-credentials /root/.secrets/godaddy.ini \
  -d "adscr.io" \
  -d "*.adscr.io"

# GoDaddy API credentials file format:
# /root/.secrets/godaddy.ini:
# dns_godaddy_key = YOUR_API_KEY
# dns_godaddy_secret = YOUR_API_SECRET
```

### Option B: Individual Certificates

```bash
# Get cert for each subdomain
certbot certonly --standalone \
  -d adscr.io \
  -d afterdark.adscr.io

# Update registry TLS config
nano /root/ads-registry/config.production.json

# Update:
"tls": {
  "enabled": true,
  "cert_file": "/etc/letsencrypt/live/adscr.io/fullchain.pem",
  "key_file": "/etc/letsencrypt/live/adscr.io/privkey.pem"
}
```

---

## Quick Reference

**Your Tenants:**

| Tenant | Domain | Schema | Status |
|--------|--------|--------|--------|
| default | `adscr.io` | public | Active |
| afterdark | `afterdark.adscr.io` | tenant_afterdark | Active |

**Create New Tenant:**

```bash
# SSH to server
docker exec -i letsgoout-postgres psql -U ads_registry -d ads_registry <<SQL
INSERT INTO tenants (slug, name, schema_name, status, contact_email)
VALUES ('acme', 'Acme Corporation', 'tenant_acme', 'active', 'admin@acme.com')
RETURNING id;

SELECT provision_tenant_schema('tenant_acme');
SQL

# New tenant will be live at: acme.adscr.io
```

---

## Troubleshooting

**DNS not resolving?**
```bash
# Check if wildcard DNS is working
dig adscr.io
dig afterdark.adscr.io
dig test.adscr.io
nslookup test.adscr.io
```

**Tenant not found?**
```bash
# Check tenant exists
docker exec letsgoout-postgres psql -U ads_registry -d ads_registry \
  -c "SELECT * FROM tenants;"

# Check registry is using correct base domain
journalctl -u ads-registry.service | grep -i "multi-tenancy"
```

**Tables missing in tenant schema?**
```bash
# Reprovision
docker exec letsgoout-postgres psql -U ads_registry -d ads_registry \
  -c "SELECT provision_tenant_schema('tenant_SLUG');"
```

---

**Need help?** The multi-tenancy system is fully deployed and ready! Just add the DNS record and you're live! 🚀
