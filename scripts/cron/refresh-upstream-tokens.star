#!/usr/bin/env ads-registry cron
# cron: 0 */6 * * *
# description: Refresh upstream registry authentication tokens every 6 hours
#
# This cron job automatically refreshes tokens for upstream registries that
# have short-lived tokens (AWS ECR tokens expire after 12 hours).
#
# Runs every 6 hours to ensure tokens are always fresh.

def on_scheduled(event):
    """Called by cron scheduler when job runs"""

    print("🔄 Starting upstream token refresh job...")
    print(f"Triggered at: {event.data['started_at']}")

    # Get all configured upstreams
    upstreams = registry.list_upstreams()

    if len(upstreams) == 0:
        print("ℹ️  No upstream registries configured")
        return

    print(f"Found {len(upstreams)} upstream registries")

    refreshed = 0
    failed = 0

    for upstream in upstreams:
        upstream_name = upstream['name']
        upstream_type = upstream['type']

        print(f"\n📦 Processing upstream: {upstream_name} ({upstream_type})")

        # Check if token needs refresh
        if upstream_type == "aws":
            # AWS ECR tokens expire after 12 hours
            # Refresh if token expires within next 6 hours
            expiry = upstream.get('token_expiry')
            if expiry:
                hours_until_expiry = (expiry - time.now()).hours

                if hours_until_expiry < 6:
                    print(f"  ⚠️  Token expires in {hours_until_expiry:.1f} hours, refreshing...")

                    try:
                        # Call AWS ECR to get new token
                        new_token = aws.get_ecr_token(
                            region=upstream['region'],
                            account_id=upstream['account_id']
                        )

                        # Update upstream with new token
                        registry.update_upstream_token(
                            upstream_id=upstream['id'],
                            token=new_token['token'],
                            expiry=new_token['expiry']
                        )

                        print(f"  ✅ Token refreshed, expires at {new_token['expiry']}")
                        refreshed += 1

                    except Exception as e:
                        print(f"  ❌ Failed to refresh token: {e}")
                        failed += 1
                else:
                    print(f"  ✓ Token still valid for {hours_until_expiry:.1f} hours")

        elif upstream_type == "oracle":
            # Oracle OCI uses API keys which don't expire
            # But we can verify they're still valid
            print("  ✓ Oracle OCI uses long-lived API keys, no refresh needed")

        elif upstream_type == "dockerhub":
            # Docker Hub tokens don't expire if using PAT
            print("  ✓ Docker Hub using personal access token, no refresh needed")

        else:
            print(f"  ⚠️  Unknown upstream type: {upstream_type}")

    # Summary
    print(f"\n📊 Refresh Summary:")
    print(f"  Refreshed: {refreshed}")
    print(f"  Failed: {failed}")
    print(f"  Skipped: {len(upstreams) - refreshed - failed}")

    if failed > 0:
        # Send alert
        http_post(
            "https://alerts.example.com/webhook",
            json.dumps({
                "title": "Upstream Token Refresh Failed",
                "message": f"{failed} upstream registries failed to refresh tokens",
                "severity": "warning"
            })
        )
