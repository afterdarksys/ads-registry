#!/usr/bin/env ads-registry cron
# cron: 0 2 * * *
# description: Clean up old images daily at 2 AM
#
# This cron job removes:
# - Untagged images older than 7 days
# - Images tagged as 'dev' or 'test' older than 30 days
# - Scan caches older than 90 days

def on_scheduled(event):
    """Called by cron scheduler when job runs"""

    print("🧹 Starting daily cleanup job...")
    print(f"Started at: {event.data['started_at']}")

    # Configuration
    UNTAGGED_MAX_AGE_DAYS = 7
    DEV_IMAGE_MAX_AGE_DAYS = 30
    SCAN_CACHE_MAX_AGE_DAYS = 90
    DRY_RUN = False  # Set to True to see what would be deleted without deleting

    if DRY_RUN:
        print("⚠️  DRY RUN MODE - No images will be deleted")

    deleted_count = 0
    freed_bytes = 0

    # Get all images
    images = registry.list_images()
    print(f"Found {len(images)} images to evaluate")

    for image in images:
        digest = image['digest']
        age_days = image['age_days']
        tags = image.get_tags(digest)
        size_bytes = image['size']

        should_delete = False
        reason = ""

        # Rule 1: Untagged images older than 7 days
        if len(tags) == 0 and age_days > UNTAGGED_MAX_AGE_DAYS:
            should_delete = True
            reason = f"untagged and {age_days} days old (max: {UNTAGGED_MAX_AGE_DAYS})"

        # Rule 2: Dev/test images older than 30 days
        if not should_delete:
            for tag in tags:
                if tag in ['dev', 'test', 'develop', 'staging'] and age_days > DEV_IMAGE_MAX_AGE_DAYS:
                    should_delete = True
                    reason = f"tagged '{tag}' and {age_days} days old (max: {DEV_IMAGE_MAX_AGE_DAYS})"
                    break

        if should_delete:
            print(f"  🗑️  Deleting {digest[:19]}... - {reason} ({format_bytes(size_bytes)})")

            if not DRY_RUN:
                try:
                    registry.delete_image(digest)
                    deleted_count += 1
                    freed_bytes += size_bytes
                except Exception as e:
                    print(f"     ❌ Failed: {e}")
            else:
                deleted_count += 1
                freed_bytes += size_bytes

    # Clean up old scan reports
    print(f"\n🔍 Cleaning up scan cache...")
    scans = registry.list_scans()
    old_scans = 0

    for scan in scans:
        scan_age_days = (time.now() - scan['created_at']).days

        if scan_age_days > SCAN_CACHE_MAX_AGE_DAYS:
            if not DRY_RUN:
                registry.delete_scan(scan['digest'], scan['scanner'])
            old_scans += 1

    print(f"  Removed {old_scans} old scan reports")

    # Summary
    print(f"\n📊 Cleanup Summary:")
    print(f"  Images deleted: {deleted_count}")
    print(f"  Space freed: {format_bytes(freed_bytes)}")
    print(f"  Scan reports cleaned: {old_scans}")

    if DRY_RUN:
        print("\n⚠️  DRY RUN - No actual deletions performed")

    # Send notification if significant cleanup occurred
    if deleted_count > 100 or freed_bytes > 10 * 1024 * 1024 * 1024:  # 10GB
        http_post(
            "https://notifications.example.com/webhook",
            json.dumps({
                "title": "Registry Cleanup Report",
                "message": f"Deleted {deleted_count} images, freed {format_bytes(freed_bytes)}",
                "severity": "info"
            })
        )

def format_bytes(bytes):
    """Format bytes as human-readable string"""
    units = ['B', 'KB', 'MB', 'GB', 'TB']
    size = float(bytes)
    unit_index = 0

    while size >= 1024 and unit_index < len(units) - 1:
        size /= 1024
        unit_index += 1

    return f"{size:.2f} {units[unit_index]}"
