#!/usr/bin/env bash

# Exit on any error
set -euo pipefail

# Directory of this script (project root)
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BIN_DIR="$PROJECT_ROOT/build"

# List of command packages to build
CMD_PACKAGES=(
    "./cmd/ads-registry"
    "./cmd/adsradm"
    "./cmd/credential-provider"
    "./cmd/migrate-sqlite-to-postgres"
)

clean() {
    echo "Cleaning $BIN_DIR..."
    rm -rf "$BIN_DIR"
    echo "Cleaned."
}

build() {
    mkdir -p "$BIN_DIR"
    for pkg in "${CMD_PACKAGES[@]}"; do
        bin_name=$(basename "$pkg")
        echo "Building $bin_name..."
        go build -o "$BIN_DIR/$bin_name" "$pkg"
    done
    echo "All binaries built into $BIN_DIR"
}

case "${1:-}" in
    clean)
        clean
        ;;
    rebuild)
        clean
        build
        ;;
    *)
        build
        ;;
esac
