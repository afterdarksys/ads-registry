#!/bin/bash
set -euo pipefail

# Standard Docker installation script for all servers
# Supports: Ubuntu, Debian, CentOS, RHEL, Fedora, Oracle Linux

echo "=== Docker Installation Script ==="
echo "Starting at: $(date)"

# Detect OS
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    VERSION=$VERSION_ID
else
    echo "Error: Cannot detect OS"
    exit 1
fi

echo "Detected OS: $OS $VERSION"

# Check if Docker is already installed
if command -v docker &> /dev/null; then
    DOCKER_VERSION=$(docker --version)
    echo "Docker is already installed: $DOCKER_VERSION"
    echo "Ensuring Docker service is running..."
    systemctl enable docker
    systemctl start docker
    echo "Docker is ready!"
    exit 0
fi

echo "Installing Docker..."

case "$OS" in
    ubuntu|debian)
        # Update package index
        apt-get update

        # Install prerequisites
        apt-get install -y \
            ca-certificates \
            curl \
            gnupg \
            lsb-release

        # Add Docker's official GPG key
        install -m 0755 -d /etc/apt/keyrings
        curl -fsSL https://download.docker.com/linux/${OS}/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
        chmod a+r /etc/apt/keyrings/docker.gpg

        # Set up the repository
        echo \
          "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${OS} \
          $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

        # Install Docker Engine
        apt-get update
        apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
        ;;

    centos|rhel|fedora|ol)
        # Install prerequisites
        yum install -y yum-utils

        # Add Docker repository
        yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo

        # Install Docker Engine
        yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
        ;;

    *)
        echo "Error: Unsupported OS: $OS"
        exit 1
        ;;
esac

# Start and enable Docker
systemctl enable docker
systemctl start docker

# Verify installation
docker --version
docker compose version

# Test Docker
echo "Testing Docker installation..."
docker run --rm hello-world

echo ""
echo "=== Docker Installation Complete ==="
echo "Docker version: $(docker --version)"
echo "Docker Compose version: $(docker compose version)"
echo "Completed at: $(date)"
