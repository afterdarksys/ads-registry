#!/usr/bin/env ads-registry script

"""
cleanup-old-images.star

Lifecycle policy script that cleans up old, untagged, or vulnerable images.

Usage:
  ads-registry script cleanup-old-images.star

This script:
1. Lists all images in the registry
2. Identifies images meeting cleanup criteria
3. Optionally deletes them based on policy
"""

# Configuration
DRY_RUN = True  # Set to False to actually delete images
MAX_AGE_DAYS = 90  # Delete images older than 90 days
KEEP_TAGGED = True  # Keep tagged images even if old
DELETE_VULNERABLE = True  # Delete images with critical CVEs
DELETE_UNTAGGED = True  # Delete untagged images
MIN_REPLICAS = 3  # Keep at least N versions of each image

def main():
    print("=" * 60)
    print("Image Lifecycle Cleanup")
    print("=" * 60)

    if DRY_RUN:
        print("\n[DRY RUN MODE] No images will be deleted")

    # Get all images
    print("\n[1/4] Listing all images...")
    images = registry.list_images()
    print(f"      Found {len(images)} total images")

    # Categorize images for cleanup
    print("\n[2/4] Analyzing images...")
    to_delete = []
    to_keep = []

    for image in images:
        # Get image details
        manifest = image.get_manifest(image["digest"])
        tags = image.get_tags(image["digest"])
        scan_results = scan.get_scan_results(image["digest"])

        # Apply cleanup rules
        should_delete = False
        reason = ""

        # Rule 1: Untagged images
        if DELETE_UNTAGGED and len(tags) == 0:
            should_delete = True
            reason = "untagged"

        # Rule 2: Old images
        elif image["age_days"] > MAX_AGE_DAYS:
            if KEEP_TAGGED and len(tags) > 0:
                should_delete = False
                reason = "old but tagged (keeping)"
            else:
                should_delete = True
                reason = f"older than {MAX_AGE_DAYS} days"

        # Rule 3: Vulnerable images
        elif DELETE_VULNERABLE and scan_results:
            critical_cves = scan_results.get("critical_vulnerabilities", 0)
            if critical_cves > 0:
                should_delete = True
                reason = f"{critical_cves} critical CVEs"

        if should_delete:
            to_delete.append({
                "image": image["name"],
                "digest": image["digest"],
                "tags": tags,
                "age_days": image["age_days"],
                "reason": reason
            })
        else:
            to_keep.append(image)

    # Display summary
    print(f"\n      Images to keep: {len(to_keep)}")
    print(f"      Images to delete: {len(to_delete)}")

    # Show what will be deleted
    print("\n[3/4] Images marked for deletion:")
    for img in to_delete:
        print(f"\n  - {img['image']}")
        print(f"    Digest: {img['digest'][:16]}...")
        print(f"    Tags: {img['tags']}")
        print(f"    Age: {img['age_days']} days")
        print(f"    Reason: {img['reason']}")

    # Delete images
    print("\n[4/4] Executing cleanup...")
    if DRY_RUN:
        print("      [DRY RUN] Would delete {} images".format(len(to_delete)))
    else:
        deleted_count = 0
        for img in to_delete:
            try:
                registry.delete_image(img["digest"])
                deleted_count += 1
                print(f"      ✓ Deleted {img['image']}")
            except Exception as e:
                print(f"      ✗ Failed to delete {img['image']}: {e}")

        print(f"\n      Successfully deleted {deleted_count}/{len(to_delete)} images")

    print("\n" + "=" * 60)
    print("Cleanup complete!")
    print("=" * 60)

main()
