#!/bin/bash
# Install ADS Registry Kubelet Credential Provider
# This script installs and configures the kubelet credential provider for automatic token rotation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PROVIDER_URL="https://apps.afterdarksys.com:5005/downloads/ads-registry-credential-provider"
INSTALL_DIR="/usr/local/bin/credential-providers"
CONFIG_DIR="/etc/kubernetes"
TOKEN_DIR="/var/lib/kubelet"
REGISTRY_URL="${REGISTRY_URL:-apps.afterdarksys.com:5005}"
TOKEN_URL="${TOKEN_URL:-https://apps.afterdarksys.com:5005/api/v2/k8s/token}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
   echo -e "${RED}Please run as root (use sudo)${NC}"
   exit 1
fi

echo -e "${GREEN}=== ADS Registry Kubelet Credential Provider Installation ===${NC}\n"

# Step 1: Create directories
echo -e "${YELLOW}[1/6]${NC} Creating directories..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$TOKEN_DIR"
echo -e "${GREEN}✓${NC} Directories created\n"

# Step 2: Download provider binary
echo -e "${YELLOW}[2/6]${NC} Downloading credential provider binary..."
if command -v curl &> /dev/null; then
    curl -Lo "$INSTALL_DIR/ads-registry-credential-provider" "$PROVIDER_URL"
elif command -v wget &> /dev/null; then
    wget -O "$INSTALL_DIR/ads-registry-credential-provider" "$PROVIDER_URL"
else
    echo -e "${RED}Error: curl or wget required${NC}"
    exit 1
fi

chmod +x "$INSTALL_DIR/ads-registry-credential-provider"
echo -e "${GREEN}✓${NC} Provider binary downloaded\n"

# Step 3: Verify binary
echo -e "${YELLOW}[3/6]${NC} Verifying installation..."
if "$INSTALL_DIR/ads-registry-credential-provider" --version; then
    echo -e "${GREEN}✓${NC} Provider binary works correctly\n"
else
    echo -e "${RED}Error: Provider binary verification failed${NC}"
    exit 1
fi

# Step 4: Create kubelet configuration
echo -e "${YELLOW}[4/6]${NC} Creating kubelet configuration..."
cat > "$CONFIG_DIR/kubelet-credential-provider.yaml" <<EOF
apiVersion: kubelet.config.k8s.io/v1
kind: CredentialProviderConfig
providers:
  - name: ads-registry-credential-provider
    matchImages:
      - "$REGISTRY_URL"
      - "$REGISTRY_URL/*"
      - "*.$REGISTRY_URL"
      - "*.$REGISTRY_URL/*"
    defaultCacheDuration: "5m"
    apiVersion: credentialprovider.kubelet.k8s.io/v1
    env:
      - name: REGISTRY_URL
        value: "$REGISTRY_URL"
      - name: TOKEN_URL
        value: "$TOKEN_URL"
      - name: TOKEN_FILE
        value: "$TOKEN_DIR/registry-token"
      - name: CACHE_DURATION
        value: "5m"
EOF
echo -e "${GREEN}✓${NC} Kubelet configuration created\n"

# Step 5: Configure authentication token
echo -e "${YELLOW}[5/6]${NC} Setting up authentication..."
if [ -z "$REGISTRY_TOKEN" ]; then
    echo -e "${YELLOW}Please enter your registry authentication token:${NC}"
    read -s REGISTRY_TOKEN
    echo
fi

if [ -n "$REGISTRY_TOKEN" ]; then
    echo -n "$REGISTRY_TOKEN" > "$TOKEN_DIR/registry-token"
    chmod 600 "$TOKEN_DIR/registry-token"
    echo -e "${GREEN}✓${NC} Authentication token configured\n"
else
    echo -e "${YELLOW}⚠${NC}  No token provided. You'll need to create $TOKEN_DIR/registry-token manually\n"
fi

# Step 6: Update kubelet configuration
echo -e "${YELLOW}[6/6]${NC} Updating kubelet configuration..."
KUBELET_CONFIG="/var/lib/kubelet/config.yaml"

if [ -f "$KUBELET_CONFIG" ]; then
    # Backup existing config
    cp "$KUBELET_CONFIG" "${KUBELET_CONFIG}.backup.$(date +%Y%m%d-%H%M%S)"

    # Check if credential provider is already configured
    if grep -q "imageCredentialProviderConfigFile" "$KUBELET_CONFIG"; then
        echo -e "${YELLOW}⚠${NC}  Credential provider config already exists in kubelet config"
        echo -e "   You may need to merge configurations manually"
    else
        # Add credential provider configuration
        cat >> "$KUBELET_CONFIG" <<EOF

# ADS Registry Credential Provider
imageCredentialProviderConfigFile: $CONFIG_DIR/kubelet-credential-provider.yaml
imageCredentialProviderBinDir: $INSTALL_DIR
EOF
        echo -e "${GREEN}✓${NC} Kubelet configuration updated\n"
    fi
else
    echo -e "${YELLOW}⚠${NC}  Kubelet config not found at $KUBELET_CONFIG"
    echo -e "   Add these lines to your kubelet config:"
    echo -e "   ${GREEN}imageCredentialProviderConfigFile: $CONFIG_DIR/kubelet-credential-provider.yaml${NC}"
    echo -e "   ${GREEN}imageCredentialProviderBinDir: $INSTALL_DIR${NC}\n"
fi

# Restart kubelet
echo -e "${YELLOW}Restarting kubelet...${NC}"
if systemctl is-active --quiet kubelet; then
    systemctl restart kubelet
    sleep 3
    if systemctl is-active --quiet kubelet; then
        echo -e "${GREEN}✓${NC} Kubelet restarted successfully\n"
    else
        echo -e "${RED}✗${NC} Kubelet failed to restart. Check logs: journalctl -u kubelet\n"
    fi
else
    echo -e "${YELLOW}⚠${NC}  Kubelet is not running. Start it manually: systemctl start kubelet\n"
fi

# Print summary
echo -e "${GREEN}=== Installation Complete ===${NC}\n"
echo -e "Configuration:"
echo -e "  Registry URL:       ${GREEN}$REGISTRY_URL${NC}"
echo -e "  Token URL:          ${GREEN}$TOKEN_URL${NC}"
echo -e "  Provider Binary:    ${GREEN}$INSTALL_DIR/ads-registry-credential-provider${NC}"
echo -e "  Kubelet Config:     ${GREEN}$CONFIG_DIR/kubelet-credential-provider.yaml${NC}"
echo -e "  Token File:         ${GREEN}$TOKEN_DIR/registry-token${NC}\n"

echo -e "Next Steps:"
echo -e "  1. Test image pull: ${GREEN}kubectl run test --image=$REGISTRY_URL/myapp/test:latest${NC}"
echo -e "  2. Check logs:      ${GREEN}journalctl -u kubelet -f${NC}"
echo -e "  3. Debug provider:  ${GREEN}DEBUG=1 $INSTALL_DIR/ads-registry-credential-provider${NC}\n"

echo -e "${GREEN}✓ Installation successful!${NC}"
