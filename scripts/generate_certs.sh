#!/bin/bash

# Exit on any error
set -e

# Define directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
CERTS_DIR="$ROOT_DIR/certs"

# Create certs directory if it doesn't exist
mkdir -p "$CERTS_DIR"

# File paths
CERT_FILE="$CERTS_DIR/server.crt"
KEY_FILE="$CERTS_DIR/server.key"

echo "Creating self-signed TLS certificates for local development..."

# Generate a self-signed certificate (valid for 365 days)
openssl req -newkey rsa:2048 -nodes -keyout "$KEY_FILE" -x509 -days 365 -out "$CERT_FILE" -subj "/C=US/ST=State/L=City/O=Organization/CN=localhost"

# Set permissions
chmod 600 "$KEY_FILE"
chmod 644 "$CERT_FILE"

echo "Certificates successfully generated!"
echo "Certificate relative path: certs/server.crt"
echo "Private key relative path: certs/server.key"
