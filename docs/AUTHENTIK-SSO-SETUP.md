# Authentik SSO Integration

This document describes how to configure the ADS Registry to use Authentik for Single Sign-On (SSO) authentication.

## Prerequisites

- Authentik server running and accessible
- Admin access to Authentik
- Registry configured with HTTPS

## Authentik Configuration

### 1. Create an OAuth2/OIDC Provider

In Authentik admin:

1. Navigate to **Applications** → **Providers**
2. Click **Create**
3. Select **OAuth2/OpenID Provider**
4. Configure:
   - **Name**: `ADS Registry`
   - **Authorization flow**: default-authentication-flow (or your custom flow)
   - **Client type**: `Confidential`
   - **Client ID**: Generate or use: `ads-registry`
   - **Client Secret**: Generate a secure secret
   - **Redirect URIs**: Add your registry callback URL:
     ```
     https://registry.afterdarksys.com/oauth2/callback
     ```
   - **Signing Key**: Select your certificate
   - **Scopes**: Add `openid`, `profile`, `email`, `groups`

### 2. Create an Application

1. Navigate to **Applications** → **Applications**
2. Click **Create**
3. Configure:
   - **Name**: `ADS Container Registry`
   - **Slug**: `ads-registry`
   - **Provider**: Select the provider created above
   - **Launch URL**: `https://registry.afterdarksys.com`

### 3. Configure Groups (Optional)

Map Authentik groups to registry permissions:

- **admins** or **registry-admins** → Full admin access (wildcard scope `*`)
- Other groups → Scoped access (`repository:groupname/*:pull,push`)

## Registry Configuration

Add the following to your `config.production.json`:

```json
{
  "auth": {
    "token_issuer": "ads-registry",
    "token_service": "registry",
    "private_key_path": "/path/to/private/key",
    "public_key_path": "/path/to/public/key",
    "token_expiration": 259200000000000,
    "oidc": {
      "enabled": true,
      "issuer": "https://sso.afterdarksys.com/application/o/ads-registry/",
      "client_id": "ads-registry",
      "client_secret": "your-client-secret-from-authentik",
      "redirect_url": "https://registry.afterdarksys.com/oauth2/callback",
      "scopes": ["openid", "profile", "email", "groups"]
    }
  }
}
```

### Configuration Fields

- **`enabled`**: Enable/disable SSO (default: `false`)
- **`issuer`**: Authentik OIDC issuer URL (ends with `/application/o/<slug>/`)
- **`client_id`**: OAuth2 client ID from Authentik
- **`client_secret`**: OAuth2 client secret from Authentik
- **`redirect_url`**: Callback URL (must match Authentik configuration)
- **`scopes`**: OIDC scopes to request (default: `["openid", "profile", "email"]`)

## User Authentication Flow

1. User visits the registry UI login page
2. User clicks **"Sign in with After Dark SSO"**
3. Registry redirects to Authentik
4. User authenticates with Authentik (if not already logged in)
5. Authentik redirects back to registry with authorization code
6. Registry exchanges code for ID token
7. Registry extracts user info and group memberships
8. Registry creates or updates local user account
9. Registry generates JWT token for registry access
10. User is redirected to dashboard

## Group-Based Authorization

The registry maps Authentik groups to registry scopes:

### Admin Groups

Users in these groups get full admin access:
- `admins`
- `registry-admins`

**Scope**: `*` (wildcard - full access)

### Regular Groups

Users in other groups get scoped access:

**Group**: `developers`
**Scope**: `repository:developers/*:pull,push`

### Default Access

Users with no groups get personal namespace access:

**Username**: `jdoe@example.com`
**Scope**: `repository:jdoe@example.com/*:pull,push`

## Testing

### Test SSO Login

1. Navigate to: `https://registry.afterdarksys.com/login`
2. Click **"Sign in with After Dark SSO"**
3. Authenticate with Authentik
4. Verify redirect to dashboard

### Test Admin Access

1. Login as a user in `admins` or `registry-admins` group
2. Verify you can access all repositories
3. Verify you can manage users and settings

### Test Regular User Access

1. Login as a user in a non-admin group
2. Verify you only see repositories in your namespace/group
3. Verify limited access to admin features

## Troubleshooting

### "Invalid or expired state" Error

- State tokens expire after 5 minutes
- Try login again
- Check server time synchronization

### "Failed to verify ID token" Error

- Verify `issuer` URL is correct (must match Authentik's issuer)
- Check Authentik signing key is valid
- Verify client ID matches

### "No username in ID token claims" Error

- Ensure Authentik is sending `email` or `preferred_username` claim
- Add required scopes to OIDC configuration

### Users Not Getting Admin Access

- Verify user is in `admins` or `registry-admins` group in Authentik
- Check `groups` scope is requested
- Verify Authentik is sending group claims

## Security Considerations

1. **HTTPS Required**: SSO only works over HTTPS
2. **Secret Protection**: Store `client_secret` securely (use Vault if enabled)
3. **State Validation**: Registry validates CSRF state tokens
4. **Token Expiry**: ID tokens and JWT tokens have limited lifetime
5. **Group Sync**: Groups are synced on each login

## Multiple SSO Providers

To support multiple authentication methods:

1. Keep `oidc.enabled = true` for SSO
2. Users can still use username/password login
3. SSO users are created automatically on first login
4. Both methods can be used interchangeably

## Disabling Username/Password Login

To enforce SSO-only authentication:

1. Remove the username/password form from the login page
2. Or configure Authentik as the only authentication method
3. Set strict password policies to prevent local account creation
