#!/bin/bash
# Check status of ADS Container Registry across all servers

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ANSIBLE_DIR="$(dirname "$SCRIPT_DIR")"
INVENTORY="${ANSIBLE_DIR}/inventory/hosts.yml"

echo -e "${YELLOW}=== ADS Container Registry Status ===${NC}"
echo ""

# Run ansible ad-hoc command to check service status
ansible registry_servers -i "$INVENTORY" -m shell \
  -a "systemctl status ads-registry --no-pager -l | head -20" \
  2>/dev/null || {
    echo -e "${RED}Failed to check service status${NC}"
    exit 1
}

echo ""
echo -e "${YELLOW}=== Registry Endpoint Check ===${NC}"
echo ""

# Check /v2/ endpoint
ansible registry_servers -i "$INVENTORY" -m shell \
  -a "curl -sI http://localhost:5005/v2/ | head -5" \
  2>/dev/null || {
    echo -e "${RED}Failed to check endpoint${NC}"
    exit 1
}

echo ""
echo -e "${YELLOW}=== Recent Logs ===${NC}"
echo ""

# Show recent logs
ansible registry_servers -i "$INVENTORY" -m shell \
  -a "journalctl -u ads-registry -n 10 --no-pager -o cat" \
  2>/dev/null || {
    echo -e "${RED}Failed to fetch logs${NC}"
    exit 1
}

echo ""
echo -e "${GREEN}Status check complete${NC}"
