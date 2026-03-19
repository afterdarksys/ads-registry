#!/bin/bash

# OCI Registry details
OCI_REGISTRY="iad.ocir.io"
OCI_NAMESPACE="idd2oizp8xvc"
DEST_REGISTRY="afterdark.adscr.io:5005"
COMPARTMENT_ID="ocid1.tenancy.oc1..aaaaaaaaiqfc57o25y3424skethbodacbasc2zy3yp2b423zj6qkhcwjkqta"
OUTPUT_FILE="oci-full-migration.txt"

echo "🔍 Generating migration list for all OCI repositories..."
echo "# OCI to ADS Registry Full Migration" > $OUTPUT_FILE
echo "# Generated: $(date)" >> $OUTPUT_FILE
echo "# Source: $OCI_REGISTRY/$OCI_NAMESPACE" >> $OUTPUT_FILE
echo "# Destination: $DEST_REGISTRY" >> $OUTPUT_FILE
echo "" >> $OUTPUT_FILE

# Get all repository names using jq
repos=$(oci artifacts container repository list \
  --compartment-id $COMPARTMENT_ID \
  --all 2>/dev/null | jq -r '.data.items[].["display-name"]')

total_repos=$(echo "$repos" | wc -l | tr -d ' ')
echo "📦 Found $total_repos repositories"

counter=0
total_images=0

for repo in $repos; do
  counter=$((counter + 1))
  echo "[$counter/$total_repos] Processing: $repo"
  
  # Get all tags for this repository
  tags=$(oci artifacts container image list \
    --compartment-id $COMPARTMENT_ID \
    --repository-name "$repo" \
    --all 2>/dev/null | jq -r '.data.items[].version' 2>/dev/null)
  
  if [ -z "$tags" ]; then
    echo "  ⚠️  No tags found, skipping"
    continue
  fi
  
  tag_count=$(echo "$tags" | wc -l | tr -d ' ')
  echo "  ✓ Found $tag_count tags"
  
  # Add each tag to migration list
  for tag in $tags; do
    echo "$OCI_REGISTRY/$OCI_NAMESPACE/$repo:$tag $DEST_REGISTRY/$repo:$tag" >> $OUTPUT_FILE
    total_images=$((total_images + 1))
  done
done

echo ""
echo "✅ Migration list generated: $OUTPUT_FILE"
echo "📊 Total repositories: $total_repos"
echo "📊 Total images (with tags): $total_images"
echo ""
echo "Next steps:"
echo "1. Review the migration list: head -20 $OUTPUT_FILE"
echo "2. Authenticate to destination: docker login afterdark.adscr.io:5005"
echo "3. Run migration: ./migrate-registry bulk-migrate-list $OUTPUT_FILE --workers 10 --continue-on-error"
