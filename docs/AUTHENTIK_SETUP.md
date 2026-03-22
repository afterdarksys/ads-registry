# Authentik OAuth2/OIDC Setup Guide for ADS Registry

This guide will help you configure Authentik to provide OAuth2/OIDC authentication for the ADS Container Registry.

## Prerequisites

- Access to Authentik admin panel at https://auth.afterdarksys.com
- Admin credentials for Authentik

## Step 1: Create OAuth2/OIDC Provider

1. **Log into Authentik:**
   - Go to https://auth.afterdarksys.com
   - Log in with your admin credentials

2. **Navigate to Providers:**
   - Click on **Admin Interface** (gear icon or admin menu)
   - Go to **Applications** → **Providers**
   - Click **Create** button

3. **Select Provider Type:**
   - Choose **OAuth2/OpenID Provider**

4. **Configure Provider Settings:**

   **Basic Settings:**
   - **Name:** `ADS Registry OAuth Provider`
   - **Authorization flow:** `default-provider-authorization-implicit-consent` (or your preferred flow)

   **Protocol Settings:**
   - **Client type:** `Confidential`
   - **Client ID:** `ads-registry` (or generate a new one)
   - **Client Secret:** Click to generate or use: `ca2ae983e7d7d4ad45e619a0da7f2079b7ec4e5510e370b0bddc45b6c6758d14`

   **Redirect URIs/Origins:**
   ```
   https://registry.afterdarksys.com/oauth2/callback
   https://registry.afterdarksys.com/ui/oauth2/callback
   ```

   **Scopes:**
   - Select: `openid`, `profile`, `email`, `groups`
   - Or add them manually in the scopes field

   **Advanced Settings:**
   - **Subject mode:** `Based on the User's UUID`
   - **Include claims in id_token:** `✓` (checked)
   - **Issuer mode:** `Per Provider` (recommended) or `Global`
   - **Token validity:**
     - Access token: `minutes=30` (default)
     - Refresh token: `days=30` (default)

5. **Save the Provider:**
   - Click **Create** or **Save**
   - **Note down the Client ID and Client Secret** - you'll need these later

6. **Get the Issuer URL:**
   - After creating, view the provider details
   - The issuer URL will be shown, typically:
     - Format: `https://auth.afterdarksys.com/application/o/ads-registry/`
     - Or: `https://auth.afterdarksys.com/application/o/<provider-slug>/`

## Step 2: Create Application

1. **Navigate to Applications:**
   - Go to **Applications** → **Applications**
   - Click **Create** button

2. **Configure Application:**
   - **Name:** `ADS Container Registry`
   - **Slug:** `ads-registry` (auto-generated or custom)
   - **Provider:** Select the provider you just created (`ADS Registry OAuth Provider`)
   - **Launch URL:** `https://registry.afterdarksys.com`
   - **Icon:** (optional) Upload a Docker/container icon

3. **Save the Application**

## Step 3: Test OIDC Discovery

Run this command to verify the OIDC configuration is accessible:

```bash
# Using the provider slug from the application
curl -s "https://auth.afterdarksys.com/application/o/ads-registry/.well-known/openid-configuration" | jq .

# Should return JSON with endpoints like:
# - authorization_endpoint
# - token_endpoint
# - userinfo_endpoint
# - jwks_uri
```

## Step 4: Configure ADS Registry

Update `/opt/ads-registry/config.json` on the production server:

```json
{
  "auth": {
    "token_issuer": "ads-registry",
    "token_service": "registry",
    "private_key_path": "",
    "public_key_path": "",
    "token_expiration": 259200000000000,
    "oidc": {
      "enabled": true,
      "issuer": "https://auth.afterdarksys.com/application/o/ads-registry/",
      "client_id": "ads-registry",
      "client_secret": "YOUR_CLIENT_SECRET_FROM_STEP_1",
      "redirect_url": "https://registry.afterdarksys.com/oauth2/callback",
      "scopes": ["openid", "profile", "email", "groups"]
    }
  }
}
```

**Important Notes:**
- The `issuer` URL must match what Authentik provides (check the OIDC discovery endpoint)
- The `client_id` must match the Client ID from the provider
- The `client_secret` must match the Client Secret from the provider
- The `redirect_url` must match what you configured in the provider's redirect URIs

## Step 5: Restart Registry

```bash
ssh root@apps.afterdarksys.com "systemctl restart ads-registry"
ssh root@apps.afterdarksys.com "systemctl status ads-registry"
```

## Step 6: Test OAuth Login

1. **Access the Registry UI:**
   - Go to https://registry.afterdarksys.com
   - Click "Sign in with SSO" or "Login"

2. **You should be redirected to Authentik:**
   - URL should be: `https://auth.afterdarksys.com/...`
   - Log in with your Authentik credentials

3. **After successful login:**
   - You should be redirected back to the registry
   - You should be logged in and see your username

## Troubleshooting

### Issue: "Not Found" or 404 when testing OIDC discovery

**Solution:** The provider slug might be different. Check in Authentik:
1. Go to the Application details
2. Look at the Launch URL or provider details
3. Try these variations:
   ```bash
   curl "https://auth.afterdarksys.com/application/o/<slug>/.well-known/openid-configuration"
   ```

### Issue: Registry fails to start with OIDC enabled

**Check logs:**
```bash
ssh root@apps.afterdarksys.com "journalctl -u ads-registry -n 100 --no-pager"
```

**Common causes:**
- Incorrect issuer URL (doesn't match Authentik's configuration)
- Invalid client secret
- Network connectivity issues to Authentik

### Issue: Redirect URI mismatch error

**Solution:** Make sure the redirect URI in the provider settings exactly matches:
- `https://registry.afterdarksys.com/oauth2/callback`

### Issue: Getting HTML instead of JSON from discovery endpoint

**Cause:** The provider doesn't exist or the URL is wrong

**Solution:**
1. Check the application slug in Authentik
2. Verify the issuer URL format
3. Ensure the provider is properly linked to the application

## Quick Reference

### Commands to Update Config

```bash
# Edit config on server
ssh root@apps.afterdarksys.com "nano /opt/ads-registry/config.json"

# Or update from local file
scp config.production.json root@apps.afterdarksys.com:/opt/ads-registry/config.json

# Restart service
ssh root@apps.afterdarksys.com "systemctl restart ads-registry"

# Check logs
ssh root@apps.afterdarksys.com "journalctl -u ads-registry -f"
```

### Verify Authentik Configuration

```bash
# Check OIDC discovery
curl -s "https://auth.afterdarksys.com/application/o/ads-registry/.well-known/openid-configuration" | jq .

# Check if provider is accessible
curl -I "https://auth.afterdarksys.com/application/o/ads-registry/"
```

## Security Notes

1. **Client Secret:** Keep this secret! Don't commit it to public repositories
2. **HTTPS Only:** Always use HTTPS for production OAuth flows
3. **Redirect URIs:** Only add trusted redirect URIs to prevent OAuth hijacking
4. **Scopes:** Only request the minimum scopes needed (openid, profile, email)

## Next Steps After Setup

Once OAuth is working:

1. **Create user accounts** in Authentik for registry access
2. **Set up groups** for role-based access control
3. **Configure policies** in Authentik to control who can access the registry
4. **Enable MFA** for additional security
5. **Monitor authentication logs** in Authentik

---

**Last Updated:** 2026-03-21
**Author:** After Dark Systems, LLC
