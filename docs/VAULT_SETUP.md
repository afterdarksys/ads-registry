# HashiCorp Vault Setup for ADS Registry

This guide shows how to configure HashiCorp Vault to store JWT signing keys for the ADS Container Registry.

## Prerequisites

- HashiCorp Vault server running and unsealed
- Vault CLI installed
- Admin token or sufficient permissions to create secrets

## Generate RSA Key Pair

First, generate an RSA key pair for JWT signing:

```bash
# Generate private key
openssl genrsa -out private.pem 4096

# Extract public key
openssl rsa -in private.pem -pubout -out public.pem
```

## Store Keys in Vault

### Enable KV Secrets Engine (if not already enabled)

```bash
vault secrets enable -path=secret kv-v2
```

### Store the Keys

```bash
# Read the keys into variables
PRIVATE_KEY=$(cat private.pem)
PUBLIC_KEY=$(cat public.pem)

# Store in Vault
vault kv put secret/ads-registry/jwt-keys \
  private_key="$PRIVATE_KEY" \
  public_key="$PUBLIC_KEY"
```

### Verify Storage

```bash
vault kv get secret/ads-registry/jwt-keys
```

## Create Vault Policy for ADS Registry

Create a policy file `ads-registry-policy.hcl`:

```hcl
# Read JWT keys
path "secret/data/ads-registry/jwt-keys" {
  capabilities = ["read"]
}

# List secrets (optional, for debugging)
path "secret/metadata/ads-registry/*" {
  capabilities = ["list"]
}
```

Apply the policy:

```bash
vault policy write ads-registry ads-registry-policy.hcl
```

## Create Vault Token for ADS Registry

```bash
vault token create \
  -policy=ads-registry \
  -period=720h \
  -display-name="ads-registry" \
  -no-default-policy
```

Copy the token from the output.

## Configure ADS Registry

Update `config.json`:

```json
{
  "vault": {
    "enabled": true,
    "address": "https://vault.company.com:8200",
    "token": "s.your-vault-token-here",
    "mount_path": "secret",
    "key_path": "ads-registry/jwt-keys"
  }
}
```

## Environment Variables (Alternative)

For better security, use environment variables instead of config file:

```bash
export VAULT_TOKEN="s.your-vault-token-here"
export VAULT_ADDR="https://vault.company.com:8200"
```

Then update config.json to use environment variables:

```json
{
  "vault": {
    "enabled": true,
    "address": "${VAULT_ADDR}",
    "token": "${VAULT_TOKEN}",
    "mount_path": "secret",
    "key_path": "ads-registry/jwt-keys"
  }
}
```

## Kubernetes Deployment with Vault

### Using Vault Agent Injector

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ads-registry
spec:
  template:
    metadata:
      annotations:
        vault.hashicorp.com/agent-inject: "true"
        vault.hashicorp.com/role: "ads-registry"
        vault.hashicorp.com/agent-inject-secret-vault-token: "auth/token/create"
        vault.hashicorp.com/agent-inject-template-vault-token: |
          {{- with secret "auth/token/create" -}}
          {{ .Auth.ClientToken }}
          {{- end }}
    spec:
      serviceAccountName: ads-registry
      containers:
      - name: registry
        image: ads-registry:latest
        env:
        - name: VAULT_TOKEN
          valueFrom:
            secretKeyRef:
              name: vault-token
              key: token
        - name: VAULT_ADDR
          value: "https://vault.vault.svc.cluster.local:8200"
```

### Using Kubernetes Secrets

If Vault Agent Injector is not available:

```bash
# Create Kubernetes secret with Vault token
kubectl create secret generic vault-token \
  --from-literal=token="s.your-vault-token-here"
```

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ads-registry
spec:
  template:
    spec:
      containers:
      - name: registry
        image: ads-registry:latest
        env:
        - name: VAULT_TOKEN
          valueFrom:
            secretKeyRef:
              name: vault-token
              key: token
        - name: VAULT_ADDR
          value: "https://vault.company.com:8200"
```

## Testing Vault Integration

Start the registry and check logs:

```bash
./ads-registry serve
```

You should see:

```
[INFO] ADS Container Registry starting up
[INFO] Vault integration enabled: https://vault.company.com:8200
[INFO] Vault health check passed
[INFO] Initialized Database: sqlite3
[INFO] Starting registry on 0.0.0.0:5005
```

## Troubleshooting

### "failed to connect to Vault"

- Check Vault address is correct
- Verify Vault is unsealed
- Check network connectivity

### "vault returned status 403"

- Verify token has correct permissions
- Check policy allows reading `secret/data/ads-registry/jwt-keys`
- Token may have expired

### "private_key not found in vault secret"

- Verify secret was stored correctly: `vault kv get secret/ads-registry/jwt-keys`
- Check key path matches config.json
- Ensure keys were stored as `private_key` and `public_key`

## Key Rotation

To rotate JWT keys:

1. Generate new key pair
2. Store new keys in Vault
3. Restart ADS Registry
4. Old tokens will be invalidated

```bash
# Generate new keys
openssl genrsa -out private-new.pem 4096
openssl rsa -in private-new.pem -pubout -out public-new.pem

# Update Vault
PRIVATE_KEY=$(cat private-new.pem)
PUBLIC_KEY=$(cat public-new.pem)

vault kv put secret/ads-registry/jwt-keys \
  private_key="$PRIVATE_KEY" \
  public_key="$PUBLIC_KEY"

# Restart registry
kubectl rollout restart deployment/ads-registry
```

## Security Best Practices

1. **Use short-lived tokens** - Set token TTL to reasonable period
2. **Rotate tokens regularly** - Automate token renewal
3. **Audit access** - Enable Vault audit logging
4. **Use TLS** - Always use HTTPS for Vault
5. **Principle of least privilege** - Grant only required permissions
6. **Monitor usage** - Track secret access in Vault logs

---

**By Ryan and the team at After Dark Systems, LLC.**
