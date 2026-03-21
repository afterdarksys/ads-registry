#!/bin/bash
set -e

# ADS Registry Build Script
# Builds all project binaries with appropriate CGO settings

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[*]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Check if Go is installed
check_go() {
    if ! command -v go &> /dev/null; then
        error "Go is not installed. Please install Go first."
    fi
    log "Go found: $(go version)"
}

# Clean previous builds
clean_builds() {
    log "Cleaning previous builds..."
    rm -f ads-registry ads-registry-linux
    rm -f adsradm
    rm -f cmd/adsradm/adsradm-linux
    rm -f migrate-registry/migrate-registry
    success "Previous builds cleaned"
}

# Run tests
run_tests() {
    log "Running unit tests..."
    if go test ./...; then
        success "Tests passed successfully"
    else
        error "Tests failed. Aborting build."
    fi
}

# Build a single binary
build_binary() {
    local name=$1
    local path=$2
    local cgo=$3
    local output=$4

    log "Building '$name'..."

    if CGO_ENABLED=$cgo go build -o "$output" "$path"; then
        success "Built '$name' -> $output"
        return 0
    else
        error "Failed to build '$name'"
        return 1
    fi
}

# Build all binaries
build_all() {
    log "Building all project binaries..."
    echo ""

    # Build ads-registry (CGO required for sqlite3)
    build_binary "ads-registry" "./cmd/ads-registry/" "1" "ads-registry"

    # Build adsradm (no CGO needed)
    build_binary "adsradm" "./cmd/adsradm/" "0" "adsradm"

    # Build migrate-registry (no CGO needed)
    mkdir -p migrate-registry
    build_binary "migrate-registry" "./migrate-registry/" "0" "migrate-registry/migrate-registry"

    echo ""
    success "All binaries built successfully!"
}

# Build Linux binaries for deployment
build_linux() {
    log "Building Linux binaries for deployment..."
    echo ""

    # Check if we can cross-compile with CGO
    local os_type=$(uname -s)
    local can_cross_compile_cgo=false

    if [ "$os_type" = "Linux" ]; then
        can_cross_compile_cgo=true
    fi

    # Build ads-registry for Linux (CGO required - needs Docker on non-Linux systems)
    if [ "$can_cross_compile_cgo" = true ]; then
        log "Building ads-registry-linux..."
        if CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o ads-registry-linux ./cmd/ads-registry/; then
            success "Built 'ads-registry-linux'"
        else
            error "Failed to build ads-registry-linux"
        fi
    else
        warn "Skipping ads-registry-linux (CGO cross-compilation requires Docker on $os_type)"
        warn "Use 'docker build' or deploy.sh for Linux binaries with CGO support"
    fi

    # Build adsradm for Linux (no CGO)
    log "Building adsradm-linux..."
    if CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/adsradm/adsradm-linux ./cmd/adsradm/; then
        success "Built 'adsradm-linux'"
    else
        error "Failed to build adsradm-linux"
    fi

    echo ""
    success "Linux binaries built (where possible)!"
}

# Show usage
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -h, --help       Show this help message"
    echo "  -c, --clean      Clean previous builds only"
    echo "  -t, --test       Run tests only"
    echo "  -l, --linux      Build Linux binaries (for deployment)"
    echo "  -s, --skip-test  Skip running tests"
    echo "  -a, --all        Build both local and Linux binaries (default)"
    echo ""
    echo "Examples:"
    echo "  $0               Build all local binaries (with tests)"
    echo "  $0 --linux       Build Linux binaries for deployment"
    echo "  $0 --skip-test   Build without running tests"
    echo "  $0 --clean       Clean previous builds"
}

# Main function
main() {
    local skip_test=false
    local clean_only=false
    local test_only=false
    local build_linux_only=false
    local build_all_flag=true

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            -c|--clean)
                clean_only=true
                build_all_flag=false
                shift
                ;;
            -t|--test)
                test_only=true
                build_all_flag=false
                shift
                ;;
            -l|--linux)
                build_linux_only=true
                build_all_flag=false
                shift
                ;;
            -s|--skip-test)
                skip_test=true
                shift
                ;;
            -a|--all)
                build_all_flag=true
                shift
                ;;
            *)
                error "Unknown option: $1. Use --help for usage information."
                ;;
        esac
    done

    log "ADS Registry Build Script"
    echo ""

    check_go

    # Clean only
    if [ "$clean_only" = true ]; then
        clean_builds
        exit 0
    fi

    # Test only
    if [ "$test_only" = true ]; then
        run_tests
        exit 0
    fi

    # Clean before building
    clean_builds
    echo ""

    # Run tests unless skipped
    if [ "$skip_test" = false ]; then
        run_tests
        echo ""
    else
        warn "Skipping tests as requested"
        echo ""
    fi

    # Build based on flags
    if [ "$build_linux_only" = true ]; then
        build_linux
    elif [ "$build_all_flag" = true ]; then
        build_all
        echo ""
        log "Also building Linux binaries..."
        echo ""
        build_linux
    else
        build_all
    fi

    echo ""
    echo "=========================================="
    echo "  Build Complete!"
    echo "=========================================="
    echo ""
    echo "Binaries:"
    echo "  Local:"
    [ -f ads-registry ] && echo "    ✓ ads-registry"
    [ -f adsradm ] && echo "    ✓ adsradm"
    [ -f migrate-registry/migrate-registry ] && echo "    ✓ migrate-registry/migrate-registry"
    echo ""
    echo "  Linux:"
    [ -f ads-registry-linux ] && echo "    ✓ ads-registry-linux"
    [ -f cmd/adsradm/adsradm-linux ] && echo "    ✓ cmd/adsradm/adsradm-linux"
    echo ""
    echo "Quick Start:"
    echo "  ./ads-registry serve          # Start the registry"
    echo "  ./adsradm --help              # Admin tool help"
    echo ""
}

# Run main function
main "$@"
