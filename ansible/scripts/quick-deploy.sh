#!/bin/bash
# Quick deployment script for ADS Container Registry
# This script builds and deploys in one command

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo "🚀 Quick deploying ADS Container Registry..."
echo ""

"$SCRIPT_DIR/deploy.sh" "$@"
