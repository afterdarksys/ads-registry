#!/bin/bash
# ADS Container Registry Deployment Script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ANSIBLE_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$ANSIBLE_DIR")"

# Default values
INVENTORY="${ANSIBLE_DIR}/inventory/hosts.yml"
PLAYBOOK="${ANSIBLE_DIR}/playbooks/deploy-registry.yml"
BUILD_BINARY="yes"
RESTART_SERVICE="yes"
VERBOSE=""

# Print usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Deploy ADS Container Registry using Ansible

OPTIONS:
    -h, --help              Show this help message
    -i, --inventory FILE    Specify inventory file (default: ${INVENTORY})
    -p, --playbook FILE     Specify playbook file (default: ${PLAYBOOK})
    -n, --no-build          Don't build binary before deployment
    -r, --no-restart        Don't restart service after deployment
    -v, --verbose           Enable verbose Ansible output
    -vv, --very-verbose     Enable very verbose Ansible output
    --check                 Run in check mode (dry-run)
    --tags TAGS             Run only tasks with specific tags
    --skip-tags TAGS        Skip tasks with specific tags

EXAMPLES:
    # Full deployment with build
    $0

    # Deploy without building binary
    $0 --no-build

    # Dry-run deployment
    $0 --check

    # Deploy to specific host
    $0 -i custom-inventory.yml

EOF
    exit 0
}

# Parse command line arguments
EXTRA_ARGS=""
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            ;;
        -i|--inventory)
            INVENTORY="$2"
            shift 2
            ;;
        -p|--playbook)
            PLAYBOOK="$2"
            shift 2
            ;;
        -n|--no-build)
            BUILD_BINARY="no"
            shift
            ;;
        -r|--no-restart)
            RESTART_SERVICE="no"
            shift
            ;;
        -v|--verbose)
            VERBOSE="-v"
            shift
            ;;
        -vv|--very-verbose)
            VERBOSE="-vv"
            shift
            ;;
        --check)
            EXTRA_ARGS="$EXTRA_ARGS --check"
            shift
            ;;
        --tags)
            EXTRA_ARGS="$EXTRA_ARGS --tags $2"
            shift 2
            ;;
        --skip-tags)
            EXTRA_ARGS="$EXTRA_ARGS --skip-tags $2"
            shift 2
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            usage
            ;;
    esac
done

# Pre-flight checks
echo -e "${YELLOW}Running pre-flight checks...${NC}"

# Check if ansible is installed
if ! command -v ansible-playbook &> /dev/null; then
    echo -e "${RED}Error: ansible-playbook not found. Please install Ansible.${NC}"
    exit 1
fi

# Check if inventory file exists
if [ ! -f "$INVENTORY" ]; then
    echo -e "${RED}Error: Inventory file not found: $INVENTORY${NC}"
    exit 1
fi

# Check if playbook file exists
if [ ! -f "$PLAYBOOK" ]; then
    echo -e "${RED}Error: Playbook file not found: $PLAYBOOK${NC}"
    exit 1
fi

# Build binary if requested
if [ "$BUILD_BINARY" = "yes" ]; then
    echo -e "${YELLOW}Building registry binary...${NC}"
    cd "$PROJECT_ROOT"
    GOOS=linux GOARCH=amd64 go build -o ads-registry-linux -ldflags="-s -w" ./cmd/ads-registry
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}Binary built successfully: ads-registry-linux${NC}"
        ls -lh ads-registry-linux
    else
        echo -e "${RED}Failed to build binary${NC}"
        exit 1
    fi
fi

# Run ansible playbook
echo -e "${YELLOW}Deploying ADS Container Registry...${NC}"
echo "Inventory: $INVENTORY"
echo "Playbook: $PLAYBOOK"
echo "Build Binary: $BUILD_BINARY"
echo "Restart Service: $RESTART_SERVICE"
echo ""

ansible-playbook \
    -i "$INVENTORY" \
    "$PLAYBOOK" \
    -e "build_binary=$BUILD_BINARY" \
    -e "restart_service=$RESTART_SERVICE" \
    $VERBOSE \
    $EXTRA_ARGS

if [ $? -eq 0 ]; then
    echo -e "${GREEN}Deployment completed successfully!${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Verify registry is running: ssh root@apps.afterdarksys.com 'systemctl status ads-registry'"
    echo "  2. Test registry endpoint: curl -I https://apps.afterdarksys.com/v2/"
    echo "  3. Push an image: docker push apps.afterdarksys.com/namespace/image:tag"
else
    echo -e "${RED}Deployment failed!${NC}"
    exit 1
fi
