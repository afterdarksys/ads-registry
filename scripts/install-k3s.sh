#!/bin/bash
set -euo pipefail

# Standard k3s installation script for all servers
# k3s is a lightweight Kubernetes distribution

echo "=== k3s Installation Script ==="
echo "Starting at: $(date)"

# Configuration
K3S_VERSION="${K3S_VERSION:-}"  # Leave empty for latest
INSTALL_K3S_EXEC="${INSTALL_K3S_EXEC:---disable=traefik}"  # Disable traefik by default

# Detect if this is a server or agent installation
INSTALL_TYPE="${1:-server}"  # server or agent

# Check if k3s is already installed
if command -v k3s &> /dev/null; then
    K3S_VERSION=$(k3s --version | head -n1)
    echo "k3s is already installed: $K3S_VERSION"
    echo "Ensuring k3s service is running..."
    systemctl enable k3s || systemctl enable k3s-agent
    systemctl start k3s || systemctl start k3s-agent
    echo "k3s is ready!"
    exit 0
fi

echo "Installing k3s ($INSTALL_TYPE mode)..."

# Install k3s
if [ "$INSTALL_TYPE" = "server" ]; then
    # Install as server (control plane + worker)
    curl -sfL https://get.k3s.io | sh -s - ${INSTALL_K3S_EXEC}

    # Wait for k3s to be ready
    echo "Waiting for k3s to be ready..."
    sleep 10

    # Set up kubeconfig for root user
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

    # Verify installation
    k3s kubectl get nodes

    echo ""
    echo "=== k3s Server Installed ==="
    echo "Kubeconfig: /etc/rancher/k3s/k3s.yaml"
    echo "To get server token for agents: sudo cat /var/lib/rancher/k3s/server/node-token"

elif [ "$INSTALL_TYPE" = "agent" ]; then
    # Install as agent (worker only)
    # Requires K3S_URL and K3S_TOKEN environment variables
    if [ -z "${K3S_URL:-}" ] || [ -z "${K3S_TOKEN:-}" ]; then
        echo "Error: K3S_URL and K3S_TOKEN must be set for agent installation"
        echo "Example: K3S_URL=https://server:6443 K3S_TOKEN=xxx ./install-k3s.sh agent"
        exit 1
    fi

    curl -sfL https://get.k3s.io | K3S_URL="$K3S_URL" K3S_TOKEN="$K3S_TOKEN" sh -

    echo ""
    echo "=== k3s Agent Installed ==="
    echo "Connected to: $K3S_URL"
else
    echo "Error: Invalid install type. Use 'server' or 'agent'"
    exit 1
fi

# Verify k3s is running
systemctl status k3s >/dev/null 2>&1 || systemctl status k3s-agent >/dev/null 2>&1

echo ""
echo "k3s version: $(k3s --version | head -n1)"
echo "Completed at: $(date)"
