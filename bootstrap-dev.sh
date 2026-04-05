#!/bin/bash
set -e

echo "=> Bootstrapping ADS Registry development environment..."

# 1. Create keys directory
mkdir -p keys

# 2. Generate RSA keys for JWT token signing
echo "=> Generating RSA 2048-bit keypair for JWT..."
if [ ! -f keys/private.key ]; then
    openssl genpkey -algorithm RSA -out keys/private.key -pkeyopt rsa_keygen_bits:2048
    openssl rsa -pubout -in keys/private.key -out keys/public.key
    echo "=> Keys generated in keys/"
else
    echo "=> Keys already exist in keys/"
fi

# 3. Update config.json to point to the keys (if jq is available)
if command -v jq >/dev/null 2>&1; then
    echo "=> Updating config.json with key paths..."
    jq '.auth.private_key_path = "keys/private.key" | .auth.public_key_path = "keys/public.key"' config.json > config.json.tmp && mv config.json.tmp config.json
else
    echo "=> NOTE: Please ensure config.json points to keys/private.key and keys/public.key manually."
fi

# 4. Build the registry
echo "=> Building ads-registry..."
go build -o ads-registry ./cmd/ads-registry

# 5. Create default admin user (Registry must not be running on SQLite, or it will be locked, but CLI can access sqlite DB directly)
echo "=> Provisioning default admin user..."
./ads-registry create-user admin --scopes="*" --password="adminPassword123" || echo "=> Admin user may already exist."

echo ""
echo "============================================="
echo "Bootstrap complete!"
echo "You can now start the server with:"
echo "./ads-registry serve --config config.json"
echo "============================================="
