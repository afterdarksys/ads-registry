#!/bin/bash
set -e

# ADS Registry Deployment Script
# For Dartnode Apps Server
# By Ryan and the team at After Dark Systems, LLC.

# Configuration
REGISTRY_NAME="ads-registry"
REGISTRY_PORT="5005"
DARTNODE_HOST="dartnode.afterdarktech.com"
DOCKER_IMAGE="ads-registry:latest"
DATA_DIR="$(pwd)/data"
LOG_FILE="deploy.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
    exit 1
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" | tee -a "$LOG_FILE"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" | tee -a "$LOG_FILE"
}

# Check if running as root (needed for Docker)
check_permissions() {
    if [ "$EUID" -eq 0 ]; then
        warn "Running as root - this is fine for deployment"
    fi
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."

    if ! command -v docker &> /dev/null; then
        error "Docker is not installed. Please install Docker first."
    fi
    success "Docker found: $(docker --version)"

    if ! command -v git &> /dev/null; then
        error "Git is not installed. Please install git first."
    fi
    success "Git found: $(git --version)"

    if ! command -v go &> /dev/null; then
        warn "Go not found - will use Docker build only"
    else
        success "Go found: $(go version)"
    fi
}

# Stop existing container
stop_existing() {
    log "Checking for existing container..."

    if docker ps -a --format '{{.Names}}' | grep -q "^${REGISTRY_NAME}$"; then
        warn "Existing container found, stopping..."
        docker stop "$REGISTRY_NAME" 2>/dev/null || true
        docker rm "$REGISTRY_NAME" 2>/dev/null || true
        success "Old container removed"
    else
        log "No existing container found"
    fi
}

# Build the Docker image
build_image() {
    log "Building Docker image..."

    if ! docker build -t "$DOCKER_IMAGE" .; then
        error "Docker build failed! Check the logs above."
    fi

    success "Docker image built successfully"
}

# Test the build
test_build() {
    log "Testing the binary build..."

    if command -v go &> /dev/null; then
        if go build -o ads-registry ./cmd/ads-registry; then
            success "Binary build test passed"
            rm -f ads-registry
        else
            error "Binary build failed!"
        fi
    else
        warn "Skipping binary build test (Go not installed)"
    fi
}

# Setup data directory
setup_data_dir() {
    log "Setting up data directory..."

    mkdir -p "$DATA_DIR/blobs"
    chmod -R 750 "$DATA_DIR"

    success "Data directory ready at: $DATA_DIR"
}

# Deploy container
deploy_container() {
    log "Deploying container to Dartnode..."

    docker run -d \
        --name "$REGISTRY_NAME" \
        --restart unless-stopped \
        -p "$REGISTRY_PORT:5005" \
        -v "$DATA_DIR:/app/data" \
        -e "REGISTRY_HOST=$DARTNODE_HOST" \
        "$DOCKER_IMAGE"

    if [ $? -eq 0 ]; then
        success "Container deployed successfully!"
    else
        error "Container deployment failed!"
    fi
}

# Wait for container to be ready
wait_for_ready() {
    log "Waiting for registry to be ready..."

    for i in {1..30}; do
        if curl -s "http://localhost:$REGISTRY_PORT/health/live" > /dev/null 2>&1; then
            success "Registry is alive!"
            break
        fi

        if [ $i -eq 30 ]; then
            error "Registry failed to start within 30 seconds"
        fi

        echo -n "."
        sleep 1
    done

    # Check readiness
    if curl -s "http://localhost:$REGISTRY_PORT/health/ready" | grep -q "ready"; then
        success "Registry is ready to accept traffic!"
    else
        warn "Registry is alive but not ready yet - checking logs..."
        docker logs --tail 20 "$REGISTRY_NAME"
    fi
}

# Create admin user
create_admin_user() {
    log "Checking for admin user..."

    # Check if we should create admin user
    read -p "Create admin user? (y/n, default: n): " CREATE_USER
    CREATE_USER=${CREATE_USER:-n}

    if [[ "$CREATE_USER" =~ ^[Yy]$ ]]; then
        log "Creating admin user..."
        docker exec -it "$REGISTRY_NAME" ./ads-registry create-user admin --scopes="*"
        success "Admin user created!"
    else
        log "Skipping admin user creation"
    fi
}

# Run tests
run_tests() {
    log "Running deployment tests..."

    # Test health endpoints
    log "Testing health endpoints..."

    LIVE=$(curl -s "http://localhost:$REGISTRY_PORT/health/live")
    if echo "$LIVE" | grep -q "alive"; then
        success "✓ Liveness check passed"
    else
        error "✗ Liveness check failed"
    fi

    READY=$(curl -s "http://localhost:$REGISTRY_PORT/health/ready")
    if echo "$READY" | grep -q "ready"; then
        success "✓ Readiness check passed"
    else
        warn "✗ Readiness check failed - registry may need database initialization"
    fi

    # Test metrics endpoint
    METRICS=$(curl -s "http://localhost:$REGISTRY_PORT/metrics")
    if echo "$METRICS" | grep -q "registry"; then
        success "✓ Metrics endpoint responding"
    else
        warn "✗ Metrics endpoint not responding properly"
    fi

    # Test v2 base endpoint
    V2=$(curl -s "http://localhost:$REGISTRY_PORT/v2/")
    if echo "$V2" | grep -q "unauthorized" || echo "$V2" | grep -q "{}"; then
        success "✓ Registry API responding"
    else
        warn "✗ Registry API response unexpected"
    fi
}

# Show deployment info
show_info() {
    echo ""
    echo "=========================================="
    echo "  ADS Registry Deployment Complete! 🚀"
    echo "=========================================="
    echo ""
    echo "Registry URL:     http://$DARTNODE_HOST:$REGISTRY_PORT"
    echo "Health Check:     http://$DARTNODE_HOST:$REGISTRY_PORT/health/ready"
    echo "Metrics:          http://$DARTNODE_HOST:$REGISTRY_PORT/metrics"
    echo "Data Directory:   $DATA_DIR"
    echo ""
    echo "Quick Commands:"
    echo "  View logs:      docker logs -f $REGISTRY_NAME"
    echo "  Stop registry:  docker stop $REGISTRY_NAME"
    echo "  Start registry: docker start $REGISTRY_NAME"
    echo "  Create user:    docker exec -it $REGISTRY_NAME ./ads-registry create-user <username> --scopes='*'"
    echo ""
    echo "Docker Login:"
    echo "  docker login $DARTNODE_HOST:$REGISTRY_PORT"
    echo ""
    echo "Push Image:"
    echo "  docker tag nginx:latest $DARTNODE_HOST:$REGISTRY_PORT/myorg/nginx:v1"
    echo "  docker push $DARTNODE_HOST:$REGISTRY_PORT/myorg/nginx:v1"
    echo ""
    echo "View Container Status:"
    docker ps | grep "$REGISTRY_NAME"
    echo ""
}

# Debug function
debug_deployment() {
    error "Deployment failed! Running diagnostics..."

    echo ""
    echo "=== Container Status ==="
    docker ps -a | grep "$REGISTRY_NAME" || echo "Container not found"

    echo ""
    echo "=== Container Logs (last 50 lines) ==="
    docker logs --tail 50 "$REGISTRY_NAME" 2>&1 || echo "No logs available"

    echo ""
    echo "=== Port Status ==="
    netstat -tuln | grep "$REGISTRY_PORT" || echo "Port not listening"

    echo ""
    echo "=== Data Directory ==="
    ls -la "$DATA_DIR" || echo "Data directory not accessible"

    echo ""
    echo "=== Disk Space ==="
    df -h "$DATA_DIR"

    echo ""
    echo "For more help, check: $LOG_FILE"
}

# Main deployment flow
main() {
    log "Starting ADS Registry deployment..."
    echo ""

    check_permissions
    check_prerequisites

    echo ""
    log "Step 1/8: Testing build..."
    test_build

    echo ""
    log "Step 2/8: Building Docker image..."
    build_image

    echo ""
    log "Step 3/8: Stopping existing container..."
    stop_existing

    echo ""
    log "Step 4/8: Setting up data directory..."
    setup_data_dir

    echo ""
    log "Step 5/8: Deploying container..."
    deploy_container

    echo ""
    log "Step 6/8: Waiting for registry to be ready..."
    wait_for_ready

    echo ""
    log "Step 7/8: Running tests..."
    run_tests

    echo ""
    log "Step 8/8: Setting up admin user..."
    create_admin_user

    echo ""
    show_info

    success "Deployment completed successfully!"
    log "Full deployment log saved to: $LOG_FILE"
}

# Trap errors
trap debug_deployment ERR

# Run main deployment
main "$@"
