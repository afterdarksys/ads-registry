#!/bin/bash
set -e

echo "=================================================="
echo "Testing Router Fix for Single and Multi-level Repos"
echo "=================================================="
echo ""

# Configuration
REGISTRY="apps.afterdarksys.com:5006"
USERNAME="admin"
PASSWORD="admin"

# Color codes for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to test a route
test_route() {
    local repo=$1
    local description=$2

    echo -n "Testing ${description}: ${repo} ... "

    # Get token for the repository
    TOKEN=$(curl -s -u "${USERNAME}:${PASSWORD}" \
        "https://${REGISTRY}/auth/token?service=registry&scope=repository:${repo}:pull,push" \
        | jq -r '.token')

    if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
        echo -e "${RED}FAILED${NC} - Could not get token"
        return 1
    fi

    # Try to start an upload (POST /v2/{repo}/blobs/uploads/)
    RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
        -H "Authorization: Bearer ${TOKEN}" \
        "https://${REGISTRY}/v2/${repo}/blobs/uploads/")

    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | head -n-1)

    if [ "$HTTP_CODE" = "202" ]; then
        echo -e "${GREEN}SUCCESS${NC} - Got 202 Accepted"
        echo "  Response headers:"
        LOCATION=$(echo "$BODY" | grep -i "Location:" || echo "  (no Location header)")
        echo "  ${LOCATION}"
        return 0
    elif [ "$HTTP_CODE" = "404" ]; then
        echo -e "${RED}FAILED${NC} - Got 404 Not Found (routing issue)"
        return 1
    elif [ "$HTTP_CODE" = "401" ]; then
        echo -e "${YELLOW}AUTH ISSUE${NC} - Got 401 Unauthorized (routing works, auth needs fixing)"
        return 0
    else
        echo -e "${RED}FAILED${NC} - Got ${HTTP_CODE}"
        echo "  Body: ${BODY}"
        return 1
    fi
}

echo "1. Testing Single-Level Repositories"
echo "-------------------------------------"
test_route "nginx" "Single-level (nginx)"
test_route "redis" "Single-level (redis)"
test_route "tokenworx" "Single-level (tokenworx)"
echo ""

echo "2. Testing Multi-Level Repositories"
echo "------------------------------------"
test_route "library/nginx" "Two-level (library/nginx)"
test_route "web3dns/aiserve-farm" "Two-level (web3dns/aiserve-farm)"
test_route "org/team/project" "Three-level (org/team/project)"
echo ""

echo "3. Testing Tags Endpoint"
echo "------------------------"
echo -n "Testing tags for tokenworx ... "
TOKEN=$(curl -s -u "${USERNAME}:${PASSWORD}" \
    "https://${REGISTRY}/auth/token?service=registry&scope=repository:tokenworx:pull" \
    | jq -r '.token')

TAGS_RESPONSE=$(curl -s -w "\n%{http_code}" \
    -H "Authorization: Bearer ${TOKEN}" \
    "https://${REGISTRY}/v2/tokenworx/tags/list")

TAGS_CODE=$(echo "$TAGS_RESPONSE" | tail -n1)
if [ "$TAGS_CODE" = "200" ] || [ "$TAGS_CODE" = "401" ]; then
    echo -e "${GREEN}SUCCESS${NC} - Got ${TAGS_CODE}"
else
    echo -e "${RED}FAILED${NC} - Got ${TAGS_CODE}"
fi

echo ""
echo "=================================================="
echo "Test Summary"
echo "=================================================="
echo "If all tests show SUCCESS or AUTH ISSUE, the routing fix is working!"
echo "404 errors indicate routing is still broken."
echo ""
