#!/usr/bin/env python3
"""
Configure Authentik OAuth2/OIDC provider for ADS Registry
Requires: pip install authentik-client
"""

import os
import sys
import json
import requests
from urllib.parse import urljoin

# Configuration
AUTHENTIK_URL = os.getenv("AUTHENTIK_URL", "https://auth.afterdarksys.com")
AUTHENTIK_TOKEN = os.getenv("AUTHENTIK_TOKEN", "")  # Set this environment variable

# OAuth Provider configuration
PROVIDER_NAME = "ADS Registry OAuth Provider"
CLIENT_ID = "ads-registry"
CLIENT_SECRET = "ca2ae983e7d7d4ad45e619a0da7f2079b7ec4e5510e370b0bddc45b6c6758d14"
REDIRECT_URIS = [
    "https://registry.afterdarksys.com/oauth2/callback",
    "https://registry.afterdarksys.com/ui/oauth2/callback",
]

# Application configuration
APP_NAME = "ADS Container Registry"
APP_SLUG = "ads-registry"
APP_LAUNCH_URL = "https://registry.afterdarksys.com"


def get_headers(token):
    """Get API headers with authentication"""
    return {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
    }


def check_provider_exists(session, provider_name):
    """Check if provider already exists"""
    url = urljoin(AUTHENTIK_URL, "/api/v3/providers/oauth2/")
    response = session.get(url)

    if response.status_code == 200:
        data = response.json()
        for provider in data.get("results", []):
            if provider["name"] == provider_name:
                print(f"✓ Provider '{provider_name}' already exists (ID: {provider['pk']})")
                return provider
    return None


def create_oauth_provider(session):
    """Create OAuth2/OIDC provider in Authentik"""
    print(f"\n📝 Creating OAuth2 provider: {PROVIDER_NAME}")

    # Check if provider exists
    existing = check_provider_exists(session, PROVIDER_NAME)
    if existing:
        return existing

    url = urljoin(AUTHENTIK_URL, "/api/v3/providers/oauth2/")

    payload = {
        "name": PROVIDER_NAME,
        "authorization_flow": "default-provider-authorization-implicit-consent",  # UUID or slug
        "client_type": "confidential",
        "client_id": CLIENT_ID,
        "client_secret": CLIENT_SECRET,
        "redirect_uris": "\n".join(REDIRECT_URIS),
        "signing_key": None,  # Will use default
        "property_mappings": [],  # Will use defaults
        "include_claims_in_id_token": True,
        "issuer_mode": "per_provider",
        "sub_mode": "hashed_user_id",
        "access_token_validity": "minutes=30",
        "refresh_token_validity": "days=30",
    }

    response = session.post(url, json=payload)

    if response.status_code in [200, 201]:
        provider = response.json()
        print(f"✓ OAuth2 provider created successfully (ID: {provider['pk']})")
        print(f"  Client ID: {provider['client_id']}")
        print(f"  Issuer URL: {AUTHENTIK_URL}/application/o/{APP_SLUG}/")
        return provider
    else:
        print(f"✗ Failed to create provider: {response.status_code}")
        print(f"  Response: {response.text}")
        return None


def check_application_exists(session, app_name):
    """Check if application already exists"""
    url = urljoin(AUTHENTIK_URL, "/api/v3/core/applications/")
    response = session.get(url)

    if response.status_code == 200:
        data = response.json()
        for app in data.get("results", []):
            if app["name"] == app_name:
                print(f"✓ Application '{app_name}' already exists (slug: {app['slug']})")
                return app
    return None


def create_application(session, provider_id):
    """Create Application in Authentik"""
    print(f"\n📝 Creating application: {APP_NAME}")

    # Check if application exists
    existing = check_application_exists(session, APP_NAME)
    if existing:
        return existing

    url = urljoin(AUTHENTIK_URL, "/api/v3/core/applications/")

    payload = {
        "name": APP_NAME,
        "slug": APP_SLUG,
        "provider": provider_id,
        "meta_launch_url": APP_LAUNCH_URL,
        "meta_description": "After Dark Systems Container Registry with OAuth2 authentication",
        "meta_publisher": "After Dark Systems, LLC",
        "policy_engine_mode": "any",
        "open_in_new_tab": True,
    }

    response = session.post(url, json=payload)

    if response.status_code in [200, 201]:
        app = response.json()
        print(f"✓ Application created successfully (slug: {app['slug']})")
        print(f"  Launch URL: {app['meta_launch_url']}")
        return app
    else:
        print(f"✗ Failed to create application: {response.status_code}")
        print(f"  Response: {response.text}")
        return None


def get_oidc_config(provider_slug):
    """Get OIDC discovery configuration"""
    url = f"{AUTHENTIK_URL}/application/o/{provider_slug}/.well-known/openid-configuration"

    try:
        response = requests.get(url)
        if response.status_code == 200:
            config = response.json()
            print(f"\n✓ OIDC Discovery endpoint working:")
            print(f"  {url}")
            return config
        else:
            print(f"\n✗ OIDC Discovery endpoint not available yet")
            return None
    except Exception as e:
        print(f"\n✗ Error accessing OIDC endpoint: {e}")
        return None


def main():
    """Main configuration function"""
    print("=" * 60)
    print("Authentik OAuth2/OIDC Configuration for ADS Registry")
    print("=" * 60)

    # Check for API token
    if not AUTHENTIK_TOKEN:
        print("\n❌ Error: AUTHENTIK_TOKEN environment variable not set")
        print("\nTo get an API token:")
        print("1. Log into Authentik at https://auth.afterdarksys.com")
        print("2. Go to Directory → Tokens & App passwords")
        print("3. Create a new token with the identifier 'api-token'")
        print("4. Copy the token and run:")
        print("   export AUTHENTIK_TOKEN='your-token-here'")
        print("   python3 scripts/configure-authentik-oauth.py")
        sys.exit(1)

    # Create session with authentication
    session = requests.Session()
    session.headers.update(get_headers(AUTHENTIK_TOKEN))

    # Test authentication
    print(f"\n🔑 Testing API authentication...")
    test_url = urljoin(AUTHENTIK_URL, "/api/v3/core/users/me/")
    response = session.get(test_url)

    if response.status_code != 200:
        print(f"✗ Authentication failed: {response.status_code}")
        print(f"  Response: {response.text}")
        print("\n  Make sure your token is valid and has admin permissions")
        sys.exit(1)

    user = response.json()
    print(f"✓ Authenticated as: {user.get('username', 'unknown')}")

    # Create OAuth provider
    provider = create_oauth_provider(session)
    if not provider:
        sys.exit(1)

    # Create application
    application = create_application(session, provider['pk'])
    if not application:
        sys.exit(1)

    # Test OIDC discovery
    get_oidc_config(APP_SLUG)

    # Print summary
    print("\n" + "=" * 60)
    print("✅ Configuration Complete!")
    print("=" * 60)
    print(f"\nOAuth2 Provider Details:")
    print(f"  Name: {PROVIDER_NAME}")
    print(f"  Client ID: {CLIENT_ID}")
    print(f"  Client Secret: {CLIENT_SECRET}")
    print(f"  Issuer URL: {AUTHENTIK_URL}/application/o/{APP_SLUG}/")
    print(f"  Redirect URIs:")
    for uri in REDIRECT_URIS:
        print(f"    - {uri}")

    print(f"\nApplication Details:")
    print(f"  Name: {APP_NAME}")
    print(f"  Slug: {APP_SLUG}")
    print(f"  Launch URL: {APP_LAUNCH_URL}")

    print(f"\nNext Steps:")
    print(f"1. Update /opt/ads-registry/config.json with OAuth settings:")
    print(f'   "oidc": {{')
    print(f'     "enabled": true,')
    print(f'     "issuer": "{AUTHENTIK_URL}/application/o/{APP_SLUG}/",')
    print(f'     "client_id": "{CLIENT_ID}",')
    print(f'     "client_secret": "{CLIENT_SECRET}",')
    print(f'     "redirect_url": "{REDIRECT_URIS[0]}",')
    print(f'     "scopes": ["openid", "profile", "email", "groups"]')
    print(f'   }}')
    print(f"\n2. Restart the registry:")
    print(f"   ssh root@apps.afterdarksys.com 'systemctl restart ads-registry'")
    print(f"\n3. Test OAuth login at:")
    print(f"   {APP_LAUNCH_URL}")
    print()


if __name__ == "__main__":
    main()
