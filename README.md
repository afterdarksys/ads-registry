# ADS Container Registry

A production-ready, OCI-compliant Docker container registry with built-in security scanning, policy enforcement, and enterprise features.

## Features

### 🔒 Security
- **JWT Authentication** - RSA-256 signed tokens with configurable expiration
- **Password Hashing** - Bcrypt with salt for secure credential storage
- **Digest Verification** - SHA256 content verification prevents corruption
- **Request Size Limits** - Protection against DoS attacks
- **Rate Limiting** - Per-IP rate limiting (100 req/min default)
- **Non-root Container** - Runs as dedicated `registry` user
- **TLS Support** - HTTPS with custom certificates

### 🐳 OCI Compliance
- **Docker Registry API v2** - Full compatibility with Docker CLI
- **Canonical JSON** - Proper manifest digest computation
- **Multi-arch Support** - Platform-specific manifest handling
- **Layer Deduplication** - Content-addressable storage

### 🛡️ Policy & Scanning
- **CEL Policy Engine** - Flexible admission control rules
- **Trivy Integration** - Automated vulnerability scanning
- **Signature Verification** - Cosign signature checking
- **Webhook Notifications** - Event-driven integrations

### 📊 Operations
- **Health Checks** - Kubernetes liveness/readiness probes
- **Prometheus Metrics** - Comprehensive observability
- **Graceful Shutdown** - Clean worker termination
- **Multi-database** - SQLite for dev, PostgreSQL for production

### 🎨 Additional Features
- **Starlark Automation** - Programmable event handlers
- **Multi-tenant** - Namespace isolation
- **Web Dashboard** - React-based admin UI (coming soon)
- **Garbage Collection** - Automatic cleanup of orphaned blobs

## Quick Start

### Prerequisites
- Docker or Go 1.22+
- PostgreSQL (recommended) or SQLite

### Installation

#### Docker (Recommended)

```bash
# Clone repository
git clone <your-repo-url>
cd ads-registry

# Build image
docker build -t ads-registry:latest .

# Run with SQLite
docker run -d \
  --name ads-registry \
  -p 5005:5005 \
  -v $(pwd)/data:/app/data \
  ads-registry:latest

# Run with PostgreSQL
docker run -d \
  --name ads-registry \
  -p 5005:5005 \
  -e USE_POSTGRES=true \
  -v $(pwd)/data:/app/data \
  ads-registry:latest
```

#### From Source

```bash
# Install dependencies
go mod download

# Build
go build -o ads-registry ./cmd/ads-registry

# Run
./ads-registry serve
```

### Create First User

```bash
# If using Docker
docker exec -it ads-registry ./ads-registry create-user admin --scopes="*"

# If running locally
./ads-registry create-user admin --scopes="*"
```

Enter a strong password when prompted.

### Test the Registry

```bash
# Check health
curl http://localhost:5005/health/ready

# Login with Docker
docker login localhost:5005
# Username: admin
# Password: <your-password>

# Push an image
docker tag nginx:latest localhost:5005/myorg/nginx:v1
docker push localhost:5005/myorg/nginx:v1

# Pull it back
docker pull localhost:5005/myorg/nginx:v1
```

## Configuration

Edit `config.json` to customize behavior:

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
    "dsn": "data/registry.db?_journal_mode=WAL&_busy_timeout=5000",
    "max_open_conns": 1,
    "max_idle_conns": 1
  },
  "storage": {
    "driver": "local",
    "local": {
      "root_directory": "data/blobs"
    }
  },
  "webhooks": [
    "http://your-webhook-endpoint.com/events"
  ]
}
```

### Environment Variables

Override config with environment variables:

- `REGISTRY_DB_DSN` - Database connection string
- `REGISTRY_TLS_CERT` - TLS certificate path
- `REGISTRY_TLS_KEY` - TLS private key path
- `USE_POSTGRES` - Enable built-in PostgreSQL (Docker only)

## Kubernetes Deployment

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
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: registry-data
```

## CLI Commands

### Server

```bash
# Start registry server
./ads-registry serve

# With custom config
./ads-registry serve --config /path/to/config.json
```

### User Management

```bash
# Create user with full access
./ads-registry create-user admin --scopes="*"

# Create user with limited access
./ads-registry create-user developer --scopes="myorg/*"
```

## Policy Enforcement

Create policies using CEL (Common Expression Language):

```go
// Example: Reject images without signatures
request.signature_is_valid == true

// Example: Block critical vulnerabilities
request.vuln_critical_count == 0

// Example: Namespace restrictions
request.namespace in ["approved-org", "trusted-vendor"]

// Example: Require specific issuer
request.signature_issuer == "security-team@company.com"
```

## API Endpoints

### Registry API (OCI)
- `GET /v2/` - Base check
- `GET /v2/<name>/manifests/<ref>` - Get manifest
- `PUT /v2/<name>/manifests/<ref>` - Push manifest
- `GET /v2/<name>/blobs/<digest>` - Get blob
- `POST /v2/<name>/blobs/uploads/` - Start upload
- `PATCH /v2/<name>/blobs/uploads/<uuid>` - Upload chunk
- `PUT /v2/<name>/blobs/uploads/<uuid>` - Complete upload

### Management
- `GET /health/live` - Liveness probe
- `GET /health/ready` - Readiness probe
- `GET /metrics` - Prometheus metrics
- `GET /auth/token` - JWT token endpoint

## Architecture

```
ads-registry/
├── cmd/
│   └── ads-registry/          # CLI entry point
│       ├── cmd/               # Cobra commands
│       └── main.go
├── internal/
│   ├── api/v2/                # OCI Registry API
│   ├── auth/                  # JWT & authentication
│   ├── automation/            # Starlark engine
│   ├── config/                # Configuration
│   ├── db/                    # Database layer
│   │   ├── sqlite/
│   │   └── postgres/
│   ├── health/                # Health checks
│   ├── policy/                # CEL policy engine
│   ├── scanner/               # Trivy integration
│   │   └── trivy/
│   ├── storage/               # Blob storage
│   │   └── local/
│   └── webhooks/              # Webhook dispatcher
├── web/                       # React dashboard (future)
├── config.json                # Default configuration
└── Dockerfile                 # Container image
```

## Performance

### Benchmarks (Preliminary)

- **Manifest Push**: ~50ms avg
- **Blob Upload**: Limited by disk I/O
- **Authentication**: ~5ms avg (JWT validation)
- **Policy Evaluation**: ~2ms avg (CEL)

### Scalability

- **SQLite**: Good for <1000 req/min, single node
- **PostgreSQL**: Scales to 10,000+ req/min, multi-node
- **Horizontal Scaling**: Stateless design allows multiple replicas

## Security

### Reporting Vulnerabilities

Please report security issues to: security@yourcompany.com

### Security Features

✅ No hardcoded credentials
✅ Password hashing with bcrypt
✅ JWT with RSA signing
✅ Content digest verification
✅ Request size limits
✅ Rate limiting
✅ Non-root container
✅ Secure file permissions

## Monitoring

### Prometheus Metrics

Available at `/metrics`:

- `registry_http_requests_total` - Request count by endpoint
- `registry_http_request_duration_seconds` - Request latency
- `registry_manifest_operations_total` - Manifest operations
- `registry_blob_operations_total` - Blob operations
- `registry_upload_size_bytes` - Upload sizes
- `registry_policy_evaluations_total` - Policy decisions
- `registry_scan_duration_seconds` - Scan durations
- `registry_vulnerabilities` - Vulnerability counts by severity

### Logging

Structured logs include:

- Request/response details
- Authentication events
- Policy decisions
- Scan results
- Errors and warnings

## Troubleshooting

### Common Issues

**Docker login fails:**
```bash
# Check health
curl http://localhost:5005/health/ready

# Verify user exists
docker exec -it ads-registry ./ads-registry create-user test --scopes="*"
```

**Database locked (SQLite):**
- Ensure only one registry instance is running
- Use PostgreSQL for production
- Check WAL mode is enabled in DSN

**Slow performance:**
- Add database indexes (automatically created)
- Use PostgreSQL instead of SQLite
- Enable caching layer
- Scale horizontally

**Out of disk space:**
- Run garbage collection
- Check blob storage usage
- Configure storage limits

## Contributing

We welcome contributions! Please see our contributing guidelines.

### Development Setup

```bash
# Clone repository
git clone <your-repo-url>
cd ads-registry

# Install dependencies
go mod download

# Run tests
go test ./...

# Build
go build -o ads-registry ./cmd/ads-registry

# Run
./ads-registry serve
```

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test ./internal/auth/...
```

## Roadmap

See [TODO.md](TODO.md) for planned features.

## License

[Your License Here]

## Credits

Built with:
- [Go](https://golang.org/) - Programming language
- [Chi](https://github.com/go-chi/chi) - HTTP router
- [CEL](https://github.com/google/cel-go) - Policy engine
- [Trivy](https://github.com/aquasecurity/trivy) - Vulnerability scanner
- [Starlark](https://github.com/google/starlark-go) - Automation engine
- [JWT](https://github.com/golang-jwt/jwt) - Authentication

## Support

- **Documentation**: [QUICKSTART.md](QUICKSTART.md)
- **Issues**: GitHub Issues
- **Discussions**: GitHub Discussions
- **Security**: security@yourcompany.com

---

**Status**: Production Ready ✅

Built for Dartnode Apps Server and enterprise deployments.

---

**The After Dark Systems Container Registry: A registry for Docker and Kubernetes that doesn't suck\!**

By Ryan and the team at After Dark Systems, LLC.
