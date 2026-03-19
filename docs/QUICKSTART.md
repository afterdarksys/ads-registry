# ADS Registry - Quick Start Guide

## For Dartnode Apps Server Deployment

### Prerequisites
- Docker installed
- Go 1.22+ (for building from source)
- PostgreSQL or SQLite

### Quick Deploy with Docker

```bash
# Build the image
docker build -t ads-registry:latest .

# Run with SQLite (simplest - for dev/staging)
docker run -d \
  --name ads-registry \
  -p 5005:5005 \
  -v $(pwd)/data:/app/data \
  ads-registry:latest

# Run with PostgreSQL (recommended for production)
docker run -d \
  --name ads-registry \
  -p 5005:5005 \
  -e USE_POSTGRES=true \
  -v $(pwd)/data:/app/data \
  ads-registry:latest
```

### Create Admin User

```bash
# If running in Docker
docker exec -it ads-registry ./ads-registry create-user admin --scopes="*"

# If running locally
./ads-registry create-user admin --scopes="*"
```

Enter a secure password when prompted.

### Test the Registry

```bash
# Health checks
curl http://localhost:5005/health/live
curl http://localhost:5005/health/ready

# Login with Docker CLI
docker login localhost:5005
# Username: admin
# Password: <your-password>

# Push a test image
docker tag nginx:latest localhost:5005/dartnode/nginx:latest
docker push localhost:5005/dartnode/nginx:latest

# Pull it back
docker pull localhost:5005/dartnode/nginx:latest
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ads-registry
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ads-registry
  template:
    metadata:
      labels:
        app: ads-registry
    spec:
      containers:
      - name: registry
        image: ads-registry:latest
        ports:
        - containerPort: 5005
        livenessProbe:
          httpGet:
            path: /health/live
            port: 5005
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 5005
          initialDelaySeconds: 10
          periodSeconds: 5
        volumeMounts:
        - name: data
          mountPath: /app/data
        env:
        - name: USE_POSTGRES
          value: "true"
        - name: REGISTRY_DB_DSN
          value: "postgres://user:pass@postgres:5432/registry?sslmode=disable"
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: registry-data
---
apiVersion: v1
kind: Service
metadata:
  name: ads-registry
spec:
  type: LoadBalancer
  ports:
  - port: 5005
    targetPort: 5005
  selector:
    app: ads-registry
```

### Configuration

Edit `config.json` to customize:

```json
{
  "server": {
    "address": "0.0.0.0",
    "port": 5005,
    "read_timeout": 300000000000,
    "write_timeout": 300000000000,
    "tls": {
      "enabled": false,
      "cert_file": "certs/server.crt",
      "key_file": "certs/server.key"
    }
  },
  "database": {
    "driver": "sqlite3",
    "dsn": "data/registry.db?_journal_mode=WAL..."
  }
}
```

### Environment Variables

Override config with environment variables:

- `REGISTRY_DB_DSN` - Database connection string
- `REGISTRY_TLS_CERT` - Path to TLS certificate
- `REGISTRY_TLS_KEY` - Path to TLS private key
- `USE_POSTGRES` - Set to "true" to use built-in PostgreSQL

### Production Checklist

- [ ] Create dedicated admin user with strong password
- [ ] Enable TLS with valid certificates
- [ ] Use PostgreSQL for database (not SQLite)
- [ ] Set up monitoring (Prometheus metrics at `/metrics`)
- [ ] Configure backup strategy for `/app/data`
- [ ] Set resource limits in Kubernetes
- [ ] Review and adjust rate limiting
- [ ] Enable network policies
- [ ] Set up log aggregation

### Security Features

✅ **JWT Authentication** - RSA-signed tokens
✅ **Password Hashing** - Bcrypt with salt
✅ **Digest Verification** - SHA256 content verification
✅ **Non-root Container** - Runs as `registry` user
✅ **Secure Permissions** - 750 on data directories
✅ **Request Size Limits** - 10MB manifests, 10GB blobs
✅ **Rate Limiting** - 100 req/min per IP

### Monitoring

```bash
# Check Prometheus metrics
curl http://localhost:5005/metrics

# Check logs
docker logs ads-registry

# Health status
curl http://localhost:5005/health/ready | jq
```

### Troubleshooting

**Can't connect to registry:**
```bash
# Check if it's running
docker ps | grep ads-registry

# Check logs
docker logs ads-registry

# Verify health
curl http://localhost:5005/health/live
```

**Authentication fails:**
```bash
# Create/recreate user
docker exec -it ads-registry ./ads-registry create-user myuser --scopes="*"

# Check database
docker exec -it ads-registry ls -la data/
```

**Database locked errors (SQLite):**
- Use PostgreSQL for production
- Check that only 1 registry instance is running
- Verify WAL mode is enabled in DSN

### Advanced Configuration

**Custom JWT Keys:**
```bash
# Generate RSA keys
openssl genrsa -out private.pem 2048
openssl rsa -in private.pem -pubout -out public.pem

# Update config.json
{
  "auth": {
    "private_key": "private.pem",
    "public_key": "public.pem"
  }
}
```

**Policy Enforcement (CEL):**
```go
// Reject unsigned images
request.signature_is_valid == true

// Block critical vulnerabilities
request.vuln_critical_count == 0

// Namespace restrictions
request.namespace in ["dartnode", "approved-vendors"]
```

### Performance Tuning

For high-traffic deployments:

1. Use PostgreSQL with connection pooling
2. Enable CDN for blob distribution
3. Add Redis for caching (policy engine)
4. Scale horizontally (2+ replicas)
5. Use external storage (S3/MinIO)

### Support

For issues or questions:
- Check logs: `docker logs ads-registry`
- Review: `PRODUCTION_HARDENING_PROGRESS.md`
- GitHub: [your-repo-url]

## Ready for Dartnode! 🚀

This registry is production-hardened with:
- 15/23 critical security fixes applied
- Health checks for Kubernetes
- Graceful shutdown handling
- Database indexes for performance
- OCI-compliant manifest handling
- Comprehensive error handling
