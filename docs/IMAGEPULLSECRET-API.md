# ImagePullSecret API

**Generate Kubernetes ImagePullSecrets with short-lived tokens**

---

## Quick Start

### Generate a Secret (One Command)

```bash
curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "name": "registry-secret",
    "namespace": "production"
  }' | jq -r '.secret' | kubectl apply -f -
```

**Done!** Your cluster can now pull images. 🚀

---

## What This Solves

### The Problem

Traditional ImagePullSecrets use **long-lived credentials**:

```yaml
# ❌ This password never expires!
apiVersion: v1
kind: Secret
metadata:
  name: regcred
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: eyJ... # Contains permanent password
```

**Problems:**
- Password leaked? Must rotate everywhere
- No expiration → permanent access
- Hard to audit who's using what

---

### The Solution

**Short-lived tokens** generated on-demand:

```bash
# Generate secret with 1-hour token
POST /api/v2/k8s/imagepullsecret

Response:
{
  "secret": "apiVersion: v1\nkind: Secret...",
  "expires_at": "2026-03-12T11:30:00Z",  // ← Token expires!
  "kubectl_command": "kubectl apply -f - <<EOF\n...\nEOF"
}
```

**Benefits:**
- ✅ Tokens expire (default: 1 hour, max: 24 hours)
- ✅ Zero-trust security
- ✅ Automatic rotation via credential provider
- ✅ Audit trail of all tokens generated

---

## API Endpoints

### 1. Generate ImagePullSecret

**POST** `/api/v2/k8s/imagepullsecret`

Generates a complete Kubernetes Secret with a short-lived token.

**Request:**

```json
{
  "name": "registry-secret",
  "namespace": "production",
  "registry": "apps.afterdarksys.com:5005",
  "username": "ryan",
  "email": "ryan@afterdarksys.com",
  "token_ttl": "1h",
  "format": "yaml"
}
```

**Parameters:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | - | Secret name |
| `namespace` | string | No | `default` | Kubernetes namespace |
| `registry` | string | No | This registry | Registry URL |
| `username` | string | No | Authenticated user | Registry username |
| `email` | string | No | User's email | Email for docker config |
| `token_ttl` | duration | No | `1h` | Token validity (max: `24h`) |
| `format` | string | No | `yaml` | Output format (`yaml` or `json`) |

**Response:**

```json
{
  "secret": "apiVersion: v1\nkind: Secret\nmetadata:\n  name: registry-secret\n  namespace: production\ntype: kubernetes.io/dockerconfigjson\ndata:\n  .dockerconfigjson: eyJhdXRocyI6eyJhcHBzLmFmdGVyZGFya3N5cy5jb206NTAwNSI6eyJ1c2VybmFtZSI6InJ5YW4iLCJwYXNzd29yZCI6ImV5SmhiR2NpT2lKU1V6STFOaUlzSW5SNWNDSTZJa3BYVkNKOS4uLiIsImVtYWlsIjoicnlhbkBhZnRlcmRhcmtzeXMuY29tIiwiYXV0aCI6ImNubGhianBsZVVwb1lXeG5hVTlLVWxOYU1rbHVJbDQzNTguLi4ifX19",
  "format": "yaml",
  "expires_at": "2026-03-12T11:30:00Z",
  "created_at": "2026-03-12T10:30:00Z",
  "kubectl_command": "kubectl apply -f - <<EOF\napiVersion: v1\nkind: Secret\n...\nEOF"
}
```

---

### 2. Generate Token Only

**POST** `/api/v2/k8s/token`

Generates just a short-lived token (no Kubernetes Secret).

**Request:**

```json
{
  "username": "ryan",
  "scopes": ["pull", "push"],
  "expires_in": "3600"
}
```

**Parameters:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `username` | string | No | Authenticated user | Username for token |
| `scopes` | []string | No | All scopes | Token permissions |
| `expires_in` | integer | No | `3600` | Seconds until expiration (max: 86400) |

**Response:**

```json
{
  "token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJyeWFuIiwiZXhwIjoxNzEwMjUyNjAwLCJzY29wZXMiOlsicHVsbCIsInB1c2giXX0.SflKxwRJ...",
  "expires_at": "2026-03-12T11:30:00Z",
  "expires_in": 3600,
  "username": "ryan"
}
```

---

### 3. Get Examples

**GET** `/api/v2/k8s/imagepullsecret/example`

Returns API documentation with examples.

---

## Usage Examples

### Example 1: One-Shot Secret Generation

```bash
# Generate and apply in one command
curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "regcred", "namespace": "default"}' \
  | jq -r '.secret' | kubectl apply -f -
```

**Output:**
```
secret/regcred created
```

---

### Example 2: Save to File

```bash
# Generate secret
curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "regcred", "namespace": "production"}' \
  | jq -r '.secret' > secret.yaml

# Review before applying
cat secret.yaml

# Apply
kubectl apply -f secret.yaml
```

---

### Example 3: JSON Format

```bash
# Generate in JSON format
curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "regcred",
    "namespace": "production",
    "format": "json"
  }' | jq -r '.secret' > secret.json

# Apply JSON
kubectl apply -f secret.json
```

---

### Example 4: Custom Token TTL

```bash
# 6-hour token (for long-running deployments)
curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "regcred-long",
    "token_ttl": "6h"
  }' | jq -r '.secret' | kubectl apply -f -
```

---

### Example 5: Use in Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      # Reference the ImagePullSecret
      imagePullSecrets:
      - name: registry-secret

      containers:
      - name: myapp
        image: apps.afterdarksys.com:5005/myapp/frontend:latest
        ports:
        - containerPort: 8080
```

---

### Example 6: Docker Login with Token

```bash
# Generate token
TOKEN_RESPONSE=$(curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/token \
  -H "Authorization: Bearer $YOUR_AUTH_TOKEN" \
  -d '{"expires_in": 3600}')

# Extract token
REGISTRY_TOKEN=$(echo $TOKEN_RESPONSE | jq -r '.token')

# Login to Docker
docker login apps.afterdarksys.com:5005 \
  -u ryan \
  -p $REGISTRY_TOKEN

# Pull image
docker pull apps.afterdarksys.com:5005/myapp/frontend:latest
```

---

### Example 7: CI/CD Integration (GitHub Actions)

```yaml
name: Deploy to Kubernetes
on: push

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Generate ImagePullSecret
        env:
          REGISTRY_TOKEN: ${{ secrets.REGISTRY_TOKEN }}
        run: |
          # Generate secret
          curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret \
            -H "Authorization: Bearer $REGISTRY_TOKEN" \
            -d '{"name": "regcred", "namespace": "production"}' \
            | jq -r '.secret' | kubectl apply -f -

      - name: Deploy application
        run: |
          kubectl apply -f deployment.yaml
          kubectl rollout status deployment/myapp
```

---

### Example 8: Automated Rotation Script

```bash
#!/bin/bash
# rotate-secrets.sh - Rotate ImagePullSecrets daily

NAMESPACES=("production" "staging" "development")

for ns in "${NAMESPACES[@]}"; do
  echo "Rotating secret in namespace: $ns"

  # Delete old secret
  kubectl delete secret regcred -n $ns --ignore-not-found

  # Generate new secret with 24-hour token
  curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret \
    -H "Authorization: Bearer $TOKEN" \
    -d "{\"name\": \"regcred\", \"namespace\": \"$ns\", \"token_ttl\": \"24h\"}" \
    | jq -r '.secret' | kubectl apply -f -

  echo "✓ Secret rotated in $ns"
done
```

**Add to cron:**
```cron
0 2 * * * /path/to/rotate-secrets.sh
```

---

## Integration with Kubelet Credential Provider

For **automatic token rotation**, use with kubelet credential provider:

**1. Install provider binary:**
```bash
curl -Lo /usr/local/bin/credential-providers/ads-registry-provider \
  https://apps.afterdarksys.com:5005/downloads/ads-registry-provider
chmod +x /usr/local/bin/credential-providers/ads-registry-provider
```

**2. Configure kubelet:**
```yaml
# /etc/kubernetes/kubelet-credential-provider-config.yaml
apiVersion: kubelet.config.k8s.io/v1
kind: CredentialProviderConfig
providers:
- name: ads-registry-provider
  matchImages:
  - "apps.afterdarksys.com:5005/*"
  defaultCacheDuration: "5m"
  apiVersion: credentialprovider.kubelet.k8s.io/v1
  args:
  - --api-url=https://apps.afterdarksys.com:5005/api/v2/k8s/token
  - --token-file=/etc/secrets/registry-token
```

**3. No ImagePullSecret needed!** 🎉

Kubelet automatically fetches fresh tokens every 5 minutes.

---

## Security Best Practices

### 1. Use Short TTLs

```bash
# ✅ Good: 1-hour token
{"token_ttl": "1h"}

# ❌ Bad: 24-hour token
{"token_ttl": "24h"}
```

**Recommendation:** 1 hour for manual secrets, 5 minutes for credential provider.

### 2. Rotate Regularly

```bash
# Automate rotation
cron: "0 */6 * * * /rotate-secrets.sh"
```

### 3. Limit Scopes

```bash
# Only grant necessary permissions
{
  "scopes": ["pull"],  // Read-only
  "expires_in": 3600
}
```

### 4. Audit Token Generation

```sql
SELECT * FROM security_audit_log
WHERE action = 'token_generated'
ORDER BY created_at DESC;
```

### 5. Use Credential Provider

For production, use kubelet credential provider instead of static secrets.

---

## Troubleshooting

### Secret Not Working

```bash
# Check secret exists
kubectl get secret regcred -n production

# Decode and inspect
kubectl get secret regcred -n production -o jsonpath='{.data.\.dockerconfigjson}' \
  | base64 -d | jq .

# Verify token hasn't expired
# (Check "exp" claim in JWT)
```

### Token Expired

```bash
# Check token expiration
TOKEN="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
echo $TOKEN | cut -d. -f2 | base64 -d | jq .exp

# Compare with current time
date +%s

# If expired, generate new secret
curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret ...
```

### Image Pull Failed

```bash
# Check pod events
kubectl describe pod POD_NAME

# Common errors:
# - "unauthorized: authentication required" → Token expired
# - "secret 'regcred' not found" → Secret not in namespace
# - "pull access denied" → Insufficient permissions
```

---

## Comparison

| Method | Expiration | Rotation | Security | Ease of Use |
|--------|------------|----------|----------|-------------|
| **Long-lived password** | Never | Manual | ❌ Low | ✅ Easy |
| **ImagePullSecret API** | 1-24 hours | Manual/Scripted | ⚠️ Medium | ✅ Easy |
| **Kubelet Credential Provider** | 5 minutes | Automatic | ✅ High | ⚠️ Complex setup |

---

## What's Next?

**Automatic Credential Provider** (coming soon):

```bash
# One-command installation
curl https://apps.afterdarksys.com:5005/install-credential-provider.sh | sudo bash

# Zero configuration needed
# All tokens auto-rotate every 5 minutes
```

---

## API Reference

See the [full API documentation](https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret/example) for complete details.

---

**Questions?** Open an [issue](https://github.com/ryan/ads-registry/issues) or check [docs](https://github.com/ryan/ads-registry/tree/main/docs).
