#!/usr/bin/env ads-registry cron
# cron: 0 0 * * 1
# description: Weekly security audit - Mondays at midnight
#
# Generates a weekly security report:
# - Images with critical/high vulnerabilities
# - Unsigned images in production
# - Missing SBOMs
# - Failed policy checks

def on_scheduled(event):
    """Weekly security audit"""

    print("🔒 Starting weekly security audit...")
    print(f"Started at: {event.data['started_at']}")

    # Get all production images
    prod_images = registry.list_images(tags_include=['prod', 'production', 'latest'])

    print(f"Auditing {len(prod_images)} production images...")

    issues = []

    for image in prod_images:
        digest = image['digest']
        repo = image['repo']
        tags = image.get_tags(digest)

        # Check 1: Critical/High CVEs
        scan = scan.get_scan_results(digest)
        if scan:
            critical = scan.get('critical_vulnerabilities', 0)
            high = scan.get('high_vulnerabilities', 0)

            if critical > 0 or high > 0:
                issues.append({
                    "severity": "high" if critical > 0 else "medium",
                    "type": "vulnerabilities",
                    "image": repo,
                    "tags": tags,
                    "message": f"{critical} critical, {high} high CVEs"
                })

        # Check 2: Signature verification
        signatures = scan.get_signatures(digest)
        if not signatures or len(signatures) == 0:
            issues.append({
                "severity": "medium",
                "type": "unsigned",
                "image": repo,
                "tags": tags,
                "message": "Image not signed with Cosign"
            })

        # Check 3: SBOM presence
        analysis = scan.analyze_supply_chain(digest)
        if not analysis['sbom']['present']:
            issues.append({
                "severity": "low",
                "type": "sbom_missing",
                "image": repo,
                "tags": tags,
                "message": "No SBOM attached"
            })

    # Generate report
    print(f"\n📋 Security Audit Report:")
    print(f"  Images audited: {len(prod_images)}")
    print(f"  Issues found: {len(issues)}")

    # Group by severity
    high_severity = [i for i in issues if i['severity'] == 'high']
    medium_severity = [i for i in issues if i['severity'] == 'medium']
    low_severity = [i for i in issues if i['severity'] == 'low']

    print(f"\n  🔴 High severity: {len(high_severity)}")
    for issue in high_severity[:10]:  # Show first 10
        print(f"    - {issue['image']}: {issue['message']}")

    print(f"\n  🟡 Medium severity: {len(medium_severity)}")
    for issue in medium_severity[:5]:
        print(f"    - {issue['image']}: {issue['message']}")

    print(f"\n  🟢 Low severity: {len(low_severity)}")

    # Send report via webhook
    report_data = {
        "title": "Weekly Security Audit Report",
        "images_audited": len(prod_images),
        "total_issues": len(issues),
        "high_severity": len(high_severity),
        "medium_severity": len(medium_severity),
        "low_severity": len(low_severity),
        "issues": issues
    }

    http_post(
        "https://security.example.com/webhook",
        json.dumps(report_data)
    )

    print(f"\n✅ Audit complete, report sent to security team")
