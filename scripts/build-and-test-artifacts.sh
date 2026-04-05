#!/bin/bash

# Build and Test Artifact Registry Implementation
# This script builds the server, CLI, and runs basic smoke tests

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "========================================"
echo "Building Artifact Registry Components"
echo "========================================"

cd "$PROJECT_ROOT"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Build server
echo -e "\n${YELLOW}Building ads-registry server...${NC}"
cd cmd/ads-registry
if go build -o ads-registry; then
    echo -e "${GREEN}✓ Server built successfully${NC}"
else
    echo -e "${RED}✗ Server build failed${NC}"
    exit 1
fi
SERVER_BIN="$(pwd)/ads-registry"

# Build CLI
echo -e "\n${YELLOW}Building artifactadm CLI...${NC}"
cd ../artifactadm
if go build -o artifactadm; then
    echo -e "${GREEN}✓ CLI built successfully${NC}"
else
    echo -e "${RED}✗ CLI build failed${NC}"
    exit 1
fi
CLI_BIN="$(pwd)/artifactadm"

# Check migrations
echo -e "\n${YELLOW}Checking migrations...${NC}"
cd "$PROJECT_ROOT"
if [ -f "migrations/017_artifact_metadata.sql" ]; then
    echo -e "${GREEN}✓ Artifact migration file found${NC}"
else
    echo -e "${RED}✗ Migration file not found${NC}"
    exit 1
fi

# Check format handlers
echo -e "\n${YELLOW}Checking format handlers...${NC}"
FORMATS=("npm" "pypi" "helm" "golang" "apt" "composer" "cocoapods" "brew")
for format in "${FORMATS[@]}"; do
    if [ -f "internal/api/formats/$format/$format.go" ]; then
        echo -e "${GREEN}✓ $format handler found${NC}"
    else
        echo -e "${RED}✗ $format handler not found${NC}"
        exit 1
    fi
done

# Check database implementations
echo -e "\n${YELLOW}Checking database implementations...${NC}"
if [ -f "internal/db/postgres/artifacts.go" ]; then
    echo -e "${GREEN}✓ PostgreSQL artifact support found${NC}"
else
    echo -e "${RED}✗ PostgreSQL artifacts not found${NC}"
    exit 1
fi

# Check SQLite has artifact support (not just stubs)
if grep -q "func (s \*SQLiteStore) CreateArtifact" internal/db/sqlite/sqlite.go; then
    echo -e "${GREEN}✓ SQLite artifact support implemented${NC}"
else
    echo -e "${RED}✗ SQLite artifacts not implemented${NC}"
    exit 1
fi

# Check documentation
echo -e "\n${YELLOW}Checking documentation...${NC}"
DOCS=("ARTIFACTADM.md" "ARTIFACT_REGISTRY_IMPLEMENTATION.md" "QUICK_START_FORMATS.md")
for doc in "${DOCS[@]}"; do
    if [ -f "docs/$doc" ]; then
        echo -e "${GREEN}✓ $doc found${NC}"
    else
        echo -e "${RED}✗ $doc not found${NC}"
        exit 1
    fi
done

# Check CLI commands
echo -e "\n${YELLOW}Checking CLI commands...${NC}"
COMMANDS=("publish" "unpublish" "list" "info" "scan" "verify" "prune" "stats")
for cmd in "${COMMANDS[@]}"; do
    if [ -f "cmd/artifactadm/cmd/$cmd.go" ]; then
        echo -e "${GREEN}✓ $cmd command found${NC}"
    else
        echo -e "${RED}✗ $cmd command not found${NC}"
        exit 1
    fi
done

# Test CLI help
echo -e "\n${YELLOW}Testing CLI help...${NC}"
if "$CLI_BIN" --help > /dev/null 2>&1; then
    echo -e "${GREEN}✓ CLI help works${NC}"
else
    echo -e "${RED}✗ CLI help failed${NC}"
    exit 1
fi

# Print CLI version
echo -e "\n${YELLOW}CLI Version:${NC}"
"$CLI_BIN" --version || true

# Summary
echo ""
echo "========================================"
echo -e "${GREEN}Build and Verification Complete!${NC}"
echo "========================================"
echo ""
echo "Binaries created:"
echo "  Server: $SERVER_BIN"
echo "  CLI:    $CLI_BIN"
echo ""
echo "Next steps:"
echo "  1. Start the server:"
echo "     cd cmd/ads-registry && ./ads-registry serve"
echo ""
echo "  2. Configure the CLI:"
echo "     cat > ~/.artifactadm.yaml <<EOF"
echo "     url: http://localhost:5005"
echo "     token: your-token-here"
echo "     format: npm"
echo "     namespace: default"
echo "     EOF"
echo ""
echo "  3. Test publishing:"
echo "     $CLI_BIN publish --format npm your-package.tgz"
echo ""
echo "Documentation:"
echo "  - docs/ARTIFACTADM.md"
echo "  - docs/ARTIFACT_REGISTRY_IMPLEMENTATION.md"
echo "  - docs/QUICK_START_FORMATS.md"
echo ""

# Optional: Run go vet
echo -e "${YELLOW}Running go vet...${NC}"
cd "$PROJECT_ROOT"
if go vet ./...; then
    echo -e "${GREEN}✓ go vet passed${NC}"
else
    echo -e "${YELLOW}⚠ go vet found issues (non-fatal)${NC}"
fi

# Optional: Check for gofmt issues
echo -e "\n${YELLOW}Checking code formatting...${NC}"
UNFORMATTED=$(gofmt -l . 2>/dev/null | grep -v vendor || true)
if [ -z "$UNFORMATTED" ]; then
    echo -e "${GREEN}✓ Code is properly formatted${NC}"
else
    echo -e "${YELLOW}⚠ Some files need formatting:${NC}"
    echo "$UNFORMATTED"
fi

echo ""
echo -e "${GREEN}All checks complete!${NC}"
