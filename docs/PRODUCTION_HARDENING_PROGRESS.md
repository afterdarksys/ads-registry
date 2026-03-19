# Production Hardening Progress Report

## Executive Summary
This document tracks the production-readiness improvements to the ADS Container Registry based on the enterprise architecture review.

**Status**: 9 of 23 Critical Issues Resolved (39% Complete)

## ✅ COMPLETED (9/23)

### Critical Security Fixes
1. **✅ Fixed SQL Syntax Errors** - Database initialization now works correctly
   - Fixed missing closing parenthesis in users table (SQLite & Postgres)
   - Added missing semicolons in schema definitions

2. **✅ Implemented Proper JWT Authentication**
   - RSA256 signature verification
   - Support for PEM key files
   - Ephemeral key generation for development
   - Proper token expiration validation
   - Files: `internal/auth/token.go`

3. **✅ Removed Hardcoded Admin Credentials**
   - Implemented bcrypt password hashing
   - Added `AuthenticateUser()`, `GetUserByUsername()`, `CreateUser()` methods
   - Created `create-user` CLI command for user management
   - Files: `internal/db/sqlite/sqlite.go`, `internal/db/postgres/postgres.go`, `cmd/ads-registry/cmd/createuser.go`

4. **✅ Added Blob Digest Verification**
   - SHA256 verification of uploaded content
   - Prevents malicious/corrupted uploads
   - Returns proper OCI error on digest mismatch
   - Automatic cleanup of invalid uploads
   - Files: `internal/api/v2/router.go:287-319`

5. **✅ Fixed Manifest Digest Computation**
   - Now uses canonical JSON (sorted keys, no whitespace)
   - Complies with OCI Distribution Specification
   - Validates JSON structure before storing
   - Files: `internal/api/v2/router.go:120-146`

### Configuration & Deployment Fixes
6. **✅ Fixed SQLite Configuration for Production**
   - Enabled WAL mode for better concurrency
   - Set proper timeouts (5 seconds busy timeout)
   - Limited connections to 1 (SQLite requirement)
   - Added cache and synchronous settings
   - Files: `config.json`

7. **✅ Fixed Docker Entrypoint Security**
   - Removed world-writable permissions (777 → 750)
   - Added non-root `registry` user
   - Proper ownership of data directories
   - Uses `gosu` for privilege dropping
   - Files: `Dockerfile`, `entrypoint.sh`

8. **✅ Fixed Timeout Configuration**
   - Increased read/write timeouts to 5 minutes (was 10 seconds)
   - Supports large blob uploads
   - Idle timeout set to 2 minutes
   - Files: `config.json`

9. **✅ Fixed Dockerfile Go Version**
   - Changed from non-existent 1.24 to 1.22
   - Added non-root user support
   - Files: `Dockerfile`

## 🚧 IN PROGRESS / REMAINING (14/23)

### Critical Priority (Must Complete Before Production)
- [ ] **Fix blob upload race condition** - Implement distributed locking
- [ ] **Implement database transactions** - Add transaction support to Store interface
- [ ] **Add database indexes** - Critical for query performance at scale

### High Priority (Performance & Reliability)
- [ ] **Add caching layer to policy engine** - Prevent DB queries in hot path
- [ ] **Implement graceful shutdown** - Stop workers cleanly on SIGTERM
- [ ] **Add health check endpoints** - For Kubernetes liveness/readiness probes
- [ ] **Implement Prometheus metrics** - Observability for production

### Medium Priority (Operational Excellence)
- [ ] **Add webhook retry logic** - Exponential backoff for failed deliveries
- [ ] **Sandbox Starlark automation** - Prevent SSRF attacks
- [ ] **Implement persistent scan queue** - Prevent scan job loss
- [ ] **Add blob garbage collection** - Prevent storage leaks
- [ ] **Fix namespace/repo path parsing** - Currently hardcoded to "library"
- [ ] **Add request size limits** - Prevent memory exhaustion
- [ ] **Implement operation-specific rate limiting** - Better DoS protection

## Key Improvements Made

### Security
- ✅ Proper JWT authentication with RSA signing
- ✅ Bcrypt password hashing
- ✅ Digest verification prevents data corruption
- ✅ Non-root container user
- ✅ Secure file permissions (750 instead of 777)

### OCI Compliance
- ✅ Canonical JSON for manifest digests
- ✅ SHA256 verification of blob uploads
- ✅ Proper error response format

### Performance
- ✅ SQLite configured with WAL mode
- ✅ Proper connection pooling (1 writer for SQLite)
- ✅ Timeouts support large uploads

### Developer Experience
- ✅ `create-user` CLI command for user management
- ✅ Environment variable overrides for config
- ✅ Development mode with ephemeral JWT keys

## Usage Instructions

### Creating Your First User
```bash
# Build the application
go build -o ads-registry ./cmd/ads-registry

# Create an admin user
./ads-registry create-user admin --scopes="*"
# Enter password when prompted

# Start the registry
./ads-registry serve
```

### Docker Deployment
```bash
# Build image
docker build -t ads-registry:latest .

# Run with SQLite (default)
docker run -p 5005:5005 -v $(pwd)/data:/app/data ads-registry:latest

# Run with PostgreSQL
docker run -p 5005:5005 \
  -e USE_POSTGRES=true \
  -v $(pwd)/data:/app/data \
  ads-registry:latest
```

### Creating Users in Docker
```bash
docker exec -it <container-id> ./ads-registry create-user myuser --scopes="*"
```

## Next Steps

### Immediate (This Session)
1. Add database indexes for performance
2. Implement health check endpoints
3. Fix namespace/repo parsing
4. Add request size limits

### Short Term (Next Sprint)
5. Implement distributed locking for uploads
6. Add comprehensive Prometheus metrics
7. Implement graceful shutdown
8. Add policy engine caching

### Medium Term
9. Webhook retry logic
10. Scan queue persistence
11. Garbage collection system
12. Starlark sandboxing

## Dependencies Added
- `golang.org/x/crypto` - For bcrypt password hashing
- `gosu` (Docker) - For privilege dropping in containers

## Breaking Changes
⚠️ **Authentication**: Hardcoded `admin/admin` credentials removed. Users must be created via `create-user` command.

## Testing Recommendations
1. Test user creation and authentication flow
2. Verify JWT token generation and validation
3. Test blob upload with digest verification
4. Verify manifest storage with canonical JSON
5. Test large file uploads (>100MB)
6. Load test with concurrent uploads
7. Test Docker container with non-root user

## Production Readiness Checklist
- [x] SQL syntax errors fixed
- [x] Authentication system secured
- [x] Blob integrity verification
- [x] Manifest digest compliance
- [x] Docker security hardened
- [ ] Database indexes added
- [ ] Health checks implemented
- [ ] Metrics/observability
- [ ] Graceful shutdown
- [ ] Race conditions resolved

**Estimated Remaining Work**: 16-20 hours for critical items, 40+ hours for full production readiness
