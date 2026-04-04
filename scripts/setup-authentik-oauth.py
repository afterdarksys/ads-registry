#!/usr/bin/env python3
"""
Configure Authentik OAuth2/OIDC provider for ADS Registry
Runs directly in the Authentik pod via kubectl exec
"""

from authentik.core.models import User
from authentik.providers.oauth2.models import OAuth2Provider
from authentik.core.models import Application
from authentik.flows.models import Flow

# Configuration
PROVIDER_NAME = "ADS Registry OAuth Provider"
CLIENT_ID = "ads-registry"
CLIENT_SECRET = "ca2ae983e7d7d4ad45e619a0da7f2079b7ec4e5510e370b0bddc45b6c6758d14"
REDIRECT_URIS = """https://registry.afterdarksys.com/oauth2/callback
https://registry.afterdarksys.com/ui/oauth2/callback"""

APP_NAME = "ADS Container Registry"
APP_SLUG = "ads-registry"
APP_LAUNCH_URL = "https://registry.afterdarksys.com"

print("=" * 60)
print("Authentik OAuth2/OIDC Configuration for ADS Registry")
print("=" * 60)

# Get default authorization flow
try:
    auth_flow = Flow.objects.filter(
        slug="default-provider-authorization-implicit-consent"
    ).first()

    if not auth_flow:
        auth_flow = Flow.objects.filter(designation="authorization").first()

    if not auth_flow:
        print("ERROR: No authorization flow found")
        exit(1)

    print(f"\n✓ Using authorization flow: {auth_flow.slug}")
except Exception as e:
    print(f"ERROR: Failed to get authorization flow: {e}")
    exit(1)

# Check if provider exists
provider = OAuth2Provider.objects.filter(name=PROVIDER_NAME).first()

if provider:
    print(f"\n✓ Provider '{PROVIDER_NAME}' already exists (ID: {provider.pk})")
    print(f"  Client ID: {provider.client_id}")
else:
    # Create OAuth2 provider
    try:
        provider = OAuth2Provider.objects.create(
            name=PROVIDER_NAME,
            authorization_flow=auth_flow,
            client_type="confidential",
            client_id=CLIENT_ID,
            client_secret=CLIENT_SECRET,
            redirect_uris=REDIRECT_URIS,
            include_claims_in_id_token=True,
            issuer_mode="per_provider",
            sub_mode="hashed_user_id",
            access_token_validity="minutes=30",
            refresh_token_validity="days=30",
        )
        print(f"\n✓ OAuth2 provider created successfully (ID: {provider.pk})")
        print(f"  Client ID: {provider.client_id}")
        print(f"  Redirect URIs:")
        for uri in REDIRECT_URIS.split('\n'):
            print(f"    - {uri}")
    except Exception as e:
        print(f"\n✗ Failed to create provider: {e}")
        exit(1)

# Check if application exists
app = Application.objects.filter(slug=APP_SLUG).first()

if app:
    print(f"\n✓ Application '{APP_NAME}' already exists (slug: {app.slug})")
    # Update provider if needed
    if app.provider != provider:
        app.provider = provider
        app.save()
        print(f"  Updated to use provider: {provider.name}")
else:
    # Create application
    try:
        app = Application.objects.create(
            name=APP_NAME,
            slug=APP_SLUG,
            provider=provider,
            meta_launch_url=APP_LAUNCH_URL,
            meta_description="After Dark Systems Container Registry with OAuth2 authentication",
            meta_publisher="After Dark Systems, LLC",
            policy_engine_mode="any",
            open_in_new_tab=True,
        )
        print(f"\n✓ Application created successfully (slug: {app.slug})")
        print(f"  Launch URL: {app.meta_launch_url}")
    except Exception as e:
        print(f"\n✗ Failed to create application: {e}")
        exit(1)

# Print summary
print("\n" + "=" * 60)
print("✅ Configuration Complete!")
print("=" * 60)

print(f"\nOAuth2 Provider Details:")
print(f"  Name: {PROVIDER_NAME}")
print(f"  Client ID: {CLIENT_ID}")
print(f"  Client Secret: {CLIENT_SECRET}")
print(f"  Issuer URL: https://auth.afterdarksys.com/application/o/{APP_SLUG}/")
print(f"  Redirect URIs:")
for uri in REDIRECT_URIS.split('\n'):
    print(f"    - {uri}")

print(f"\nApplication Details:")
print(f"  Name: {APP_NAME}")
print(f"  Slug: {APP_SLUG}")
print(f"  Launch URL: {APP_LAUNCH_URL}")

print(f"\nOIDC Discovery URL:")
print(f"  https://auth.afterdarksys.com/application/o/{APP_SLUG}/.well-known/openid-configuration")

print(f"\nRegistry Config (config.json):")
print(f'  "oidc": {{')
print(f'    "enabled": true,')
print(f'    "issuer": "https://auth.afterdarksys.com/application/o/{APP_SLUG}/",')
print(f'    "client_id": "{CLIENT_ID}",')
print(f'    "client_secret": "{CLIENT_SECRET}",')
print(f'    "redirect_url": "https://registry.afterdarksys.com/oauth2/callback",')
print(f'    "scopes": ["openid", "profile", "email", "groups"]')
print(f'  }}')

print("\n✅ Authentik is now configured!")
print("Next: Enable OIDC in registry config and restart the service.")
