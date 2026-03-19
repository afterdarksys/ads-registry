# 🔒 SSL Setup for adscr.io Wildcard Certificate

## Why You Need This

Docker requires valid SSL certificates when using HTTPS URLs. Since your registry uses:
- `https://adscr.io:5005/auth/token`
- `https://afterdark.adscr.io:5005/auth/token`

You need a **wildcard SSL certificate** that covers both `adscr.io` AND `*.adscr.io`.

---

## 🎯 Quick Setup (10 Minutes)

### Step 1: Start Certbot on Server

```bash
ssh root@apps.afterdarksys.com

certbot certonly \
  --manual \
  --preferred-challenges dns \
  -d 'adscr.io' \
  -d '*.adscr.io' \
  --agree-tos \
  --email coleman.ryan@gmail.com \
  --manual-public-ip-logging-ok
```

Certbot will prompt you with a DNS challenge. **Don't press Enter yet!**

---

### Step 2: Add TXT Records (From Your Local Machine)

Certbot will show you something like:

```
Please deploy a DNS TXT record under the name
_acme-challenge.adscr.io with the following value:

ABC123XYZ456... (random string)

Before continuing, verify the record is deployed.
```

**Open a NEW terminal on your local machine** and run:

```bash
# Replace ABC123XYZ456 with the actual value from certbot
CHALLENGE_VALUE="ABC123XYZ456_FROM_CERTBOT"

# Add TXT record to Oracle DNS
oci dns record domain patch \
  --zone-name-or-id "adscr.io" \
  --domain "_acme-challenge.adscr.io" \
  --scope GLOBAL \
  --items "[{\"domain\":\"_acme-challenge.adscr.io\",\"rdata\":\"${CHALLENGE_VALUE}\",\"rtype\":\"TXT\",\"ttl\":60}]"
```

Wait 30-60 seconds, then verify:

```bash
dig _acme-challenge.adscr.io TXT +short
# Should show your challenge value
```

---

### Step 3: Press Enter in Certbot

Once the TXT record is verified, go back to your SSH session and **press Enter**.

Certbot will give you a **SECOND** challenge for the wildcard domain. Repeat Step 2 with the new value!

---

### Step 4: Update Registry Config

After certbot succeeds, update the registry configuration:

```bash
# Edit production config
nano /root/ads-registry/config.production.json
```

Add the TLS section:

```json
{
  "database": {
    ...
  },
  "storage": {
    ...
  },
  "tls": {
    "enabled": true,
    "cert_file": "/etc/letsencrypt/live/adscr.io/fullchain.pem",
    "key_file": "/etc/letsencrypt/live/adscr.io/privkey.pem"
  }
}
```

Restart the service:

```bash
systemctl restart ads-registry.service
```

---

### Step 5: Test HTTPS

```bash
# Test root domain
curl https://adscr.io:5006/v2/

# Test wildcard subdomain
curl https://afterdark.adscr.io:5006/v2/
```

Both should return `401 Unauthorized` with valid SSL!

---

## 🔧 Automated Version (Using Script)

For easier automation, here's a script:

```bash
#!/bin/bash
# ssl-setup-adscr.sh

set -e

echo "Step 1: Starting certbot..."
echo "This will show DNS challenges. Keep this window open!"
echo ""

# Start certbot in background
ssh root@apps.afterdarksys.com "certbot certonly \
  --manual \
  --preferred-challenges dns \
  -d 'adscr.io' \
  -d '*.adscr.io' \
  --agree-tos \
  --email coleman.ryan@gmail.com \
  --manual-public-ip-logging-ok \
  --manual-auth-hook /root/oracle-dns-auth.sh \
  --manual-cleanup-hook /root/oracle-dns-cleanup.sh"
```

But you still need the DNS auth scripts configured on the server with OCI CLI.

---

## 🚀 Alternative: HTTP-Only for Development

If you want to test without SSL first, update the auth URLs to use HTTP:

```bash
# Temporarily use HTTP (not recommended for production)
export REGISTRY_BASE_URL=http://adscr.io:5005
systemctl restart ads-registry.service
```

Then Docker commands will work without SSL:

```bash
docker login adscr.io:5005  # Works with HTTP
docker push adscr.io:5005/myapp:latest
```

**But this is insecure!** Use SSL for production.

---

## 📋 Certificate Renewal

Let's Encrypt certs expire after 90 days. Set up auto-renewal:

```bash
# Add cron job
crontab -e

# Add this line (runs twice daily):
0 0,12 * * * certbot renew --quiet --post-hook "systemctl restart ads-registry.service"
```

---

## 🔍 Troubleshooting

### Certificate not found
```bash
ls -la /etc/letsencrypt/live/adscr.io/
```

Should show:
- `fullchain.pem`
- `privkey.pem`

### DNS challenge failing
```bash
# Verify TXT record exists
dig _acme-challenge.adscr.io TXT +short

# Should return the challenge value
```

### Wrong permissions
```bash
chmod 600 /etc/letsencrypt/live/adscr.io/privkey.pem
chmod 644 /etc/letsencrypt/live/adscr.io/fullchain.pem
```

---

## 🎯 Quick Commands Reference

```bash
# Start SSL cert request
ssh root@apps.afterdarksys.com
certbot certonly --manual --preferred-challenges dns -d 'adscr.io' -d '*.adscr.io'

# Add TXT record (from local machine)
oci dns record domain patch \
  --zone-name-or-id "adscr.io" \
  --domain "_acme-challenge.adscr.io" \
  --scope GLOBAL \
  --items '[{"domain":"_acme-challenge.adscr.io","rdata":"CHALLENGE_VALUE","rtype":"TXT","ttl":60}]'

# Verify TXT record
dig _acme-challenge.adscr.io TXT +short

# After cert is issued, update config and restart
nano /root/ads-registry/config.production.json  # Add TLS section
systemctl restart ads-registry.service

# Test HTTPS
curl https://adscr.io:5006/v2/
curl https://afterdark.adscr.io:5006/v2/
```

---

## ✅ What You Get

Once SSL is configured:

✅ Secure Docker registry (https://)
✅ Valid certificates for all subdomains (*.adscr.io)
✅ Browser-trusted connections
✅ Production-ready security
✅ Auto-renewal every 90 days

---

**Status**: Ready to set up SSL!
**Time needed**: ~10 minutes
**Manual steps**: Add 2 DNS TXT records during setup
**Auto-renews**: Yes (with cron job)
