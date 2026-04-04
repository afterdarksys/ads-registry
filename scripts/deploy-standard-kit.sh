#!/bin/bash
set -euo pipefail

# Deploy standard kit (Docker + k3s) to all servers
# Usage: ./deploy-standard-kit.sh [docker|k3s|all]

COMPONENT="${1:-all}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# List of all servers from SSH config
SERVERS=(
    "relay-b.msgs.global"
    "mx.nerdycupid.com"
    "apps.afterdarksys.com"
    "relay-a"
    "relay-b"
    "dr1"
    "apps2"
)

# Skip certain servers if needed
SKIP_SERVERS=(
    # "nerdycupid.com"  # Example: skip if not accessible
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

function should_skip() {
    local server=$1
    if [ ${#SKIP_SERVERS[@]} -eq 0 ]; then
        return 1
    fi
    for skip in "${SKIP_SERVERS[@]}"; do
        if [ "$server" = "$skip" ]; then
            return 0
        fi
    done
    return 1
}

function install_docker() {
    local server=$1
    echo -e "${YELLOW}=== Installing Docker on $server ===${NC}"

    # Copy script to server
    scp -q "$SCRIPT_DIR/install-docker.sh" "$server:/tmp/install-docker.sh"

    # Run installation
    ssh "$server" "sudo bash /tmp/install-docker.sh"

    # Cleanup
    ssh "$server" "rm /tmp/install-docker.sh"

    echo -e "${GREEN}✓ Docker installed on $server${NC}\n"
}

function install_k3s() {
    local server=$1
    echo -e "${YELLOW}=== Installing k3s on $server ===${NC}"

    # Copy script to server
    scp -q "$SCRIPT_DIR/install-k3s.sh" "$server:/tmp/install-k3s.sh"

    # Run installation (server mode)
    ssh "$server" "sudo bash /tmp/install-k3s.sh server"

    # Cleanup
    ssh "$server" "rm /tmp/install-k3s.sh"

    echo -e "${GREEN}✓ k3s installed on $server${NC}\n"
}

function verify_installation() {
    local server=$1
    echo -e "${YELLOW}Verifying $server...${NC}"

    local docker_ok=false
    local k3s_ok=false

    # Check Docker
    if ssh "$server" "command -v docker >/dev/null 2>&1"; then
        docker_version=$(ssh "$server" "docker --version 2>&1 || echo 'error'")
        if [[ "$docker_version" != "error" ]]; then
            echo -e "  ${GREEN}✓${NC} Docker: $docker_version"
            docker_ok=true
        else
            echo -e "  ${RED}✗${NC} Docker: Not installed or not working"
        fi
    else
        echo -e "  ${RED}✗${NC} Docker: Not found"
    fi

    # Check k3s
    if ssh "$server" "command -v k3s >/dev/null 2>&1"; then
        k3s_version=$(ssh "$server" "k3s --version 2>&1 | head -n1 || echo 'error'")
        if [[ "$k3s_version" != "error" ]]; then
            echo -e "  ${GREEN}✓${NC} k3s: $k3s_version"
            k3s_ok=true
        else
            echo -e "  ${RED}✗${NC} k3s: Not installed or not working"
        fi
    else
        echo -e "  ${RED}✗${NC} k3s: Not found"
    fi

    echo ""
}

# Main execution
echo "=== Standard Kit Deployment ==="
echo "Component: $COMPONENT"
echo "Servers: ${#SERVERS[@]}"
echo ""

SUCCESS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

for server in "${SERVERS[@]}"; do
    if should_skip "$server"; then
        echo -e "${YELLOW}⊘ Skipping $server (in skip list)${NC}\n"
        ((SKIP_COUNT++))
        continue
    fi

    # Test connectivity
    if ! ssh -o ConnectTimeout=5 -o BatchMode=yes "$server" "echo 'Connection OK'" >/dev/null 2>&1; then
        echo -e "${RED}✗ Cannot connect to $server (skipping)${NC}\n"
        ((FAIL_COUNT++))
        continue
    fi

    # Install components
    if [ "$COMPONENT" = "docker" ] || [ "$COMPONENT" = "all" ]; then
        if install_docker "$server"; then
            ((SUCCESS_COUNT++))
        else
            echo -e "${RED}✗ Failed to install Docker on $server${NC}\n"
            ((FAIL_COUNT++))
            continue
        fi
    fi

    if [ "$COMPONENT" = "k3s" ] || [ "$COMPONENT" = "all" ]; then
        if install_k3s "$server"; then
            ((SUCCESS_COUNT++))
        else
            echo -e "${RED}✗ Failed to install k3s on $server${NC}\n"
            ((FAIL_COUNT++))
            continue
        fi
    fi
done

echo ""
echo "=== Verification Phase ==="
echo ""

for server in "${SERVERS[@]}"; do
    if should_skip "$server"; then
        continue
    fi

    if ssh -o ConnectTimeout=5 -o BatchMode=yes "$server" "echo" >/dev/null 2>&1; then
        verify_installation "$server"
    fi
done

echo "=== Summary ==="
echo -e "${GREEN}Successful: $SUCCESS_COUNT${NC}"
echo -e "${RED}Failed: $FAIL_COUNT${NC}"
echo -e "${YELLOW}Skipped: $SKIP_COUNT${NC}"
echo ""
echo "Deployment complete!"
