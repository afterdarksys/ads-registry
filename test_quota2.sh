#!/bin/bash
TOKEN=$(curl -k -u "admin:admin" -s "https://localhost:5006/auth/token?service=registry&scope=repository:limited-user/test:push,pull" | jq -r .token)

LOC_HEADER=$(curl -k -i -X POST -H "Authorization: Bearer $TOKEN" "https://localhost:5006/v2/limited-user/test/blobs/uploads/" | grep -i Location | tr -d '\r')
UPLOAD_LOC=$(echo $LOC_HEADER | awk '{print $2}')

PAYLOAD="quota_test_random_$RANDOM"
DIGEST=$(echo -n "$PAYLOAD" | shasum -a 256 | awk '{print $1}')

echo "Uploading new random blob (${#PAYLOAD} bytes)..."
curl -k -i -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/octet-stream" -d "$PAYLOAD" "https://localhost:5006${UPLOAD_LOC}?digest=sha256:${DIGEST}"
