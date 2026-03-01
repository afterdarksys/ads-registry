# TODO - ADS Container Registry

This document tracks planned features, improvements, and known issues for the ADS Container Registry.

## Critical Priority (Next Release)

### Database & Performance
- [ ] **Implement database transactions**
  - Add Transaction interface to Store
  - Wrap multi-step operations (manifest + blob insert)
  - Add rollback support on failures
  - Files: `internal/db/db.go`, `internal/api/v2/router.go`

- [ ] **Fix blob upload race condition**
  - Implement distributed locking (Redis or PostgreSQL advisory locks)
  - Add idempotency checks (blob already exists)
  - Prevent concurrent writes to same blob
  - Files: `internal/api/v2/router.go:298-350`

- [ ] **Add policy engine caching**
  - Implement Redis cache for manifest metadata
  - Cache signature validation results (5min TTL)
  - Cache vulnerability scan results (1hr TTL)
  - Reduce DB queries in hot path from 3 to 0
  - Files: `internal/policy/cel.go:89-116`

### Testing & Quality
- [ ] **Add comprehensive test suite**
  - Unit tests for all packages (target: 80% coverage)
  - Integration tests for API endpoints
  - End-to-end Docker push/pull tests
  - Load testing for concurrent uploads
  - Security testing for auth bypass attempts

- [ ] **Add benchmarks**
  - Manifest push/pull latency
  - Authentication performance
  - Policy evaluation overhead
  - Database query performance

## High Priority (v0.2.0)

### Operations & Reliability
- [ ] **Webhook retry logic with exponential backoff**
  - Persistent queue for webhook deliveries
  - Retry with backoff (1s, 2s, 4s, 8s, 16s)
  - Dead letter queue for failed deliveries
  - Admin UI to view/retry failed webhooks
  - Files: `internal/webhooks/webhooks.go:32-69`

- [ ] **Blob garbage collection**
  - Mark-and-sweep algorithm
  - Grace period (24hr default) before deletion
  - Dry-run mode
  - CLI command: `ads-registry gc --dry-run`
  - Scheduled execution (weekly)

- [ ] **Comprehensive Prometheus metrics**
  - HTTP request metrics (method, path, status)
  - Database query duration by operation
  - Storage I/O metrics
  - Policy evaluation metrics
  - Scan duration by scanner
  - Vulnerability counts by severity
  - Upload size histogram

### Security
- [ ] **Sandbox Starlark automation**
  - Block private IP ranges (SSRF protection)
  - URL allowlist for HTTP requests
  - Execution timeout (30s max)
  - Memory limits
  - CPU limits
  - Files: `internal/automation/starlark.go:78-110`

- [ ] **Implement RBAC (Role-Based Access Control)**
  - Define roles: admin, developer, viewer
  - Per-namespace permissions
  - Per-repository permissions
  - Action-based scopes (pull, push, delete)

- [ ] **Audit logging**
  - Log all authentication attempts
  - Log policy decisions (allow/deny)
  - Log administrative actions
  - Export to external systems (Splunk, ELK)

## Medium Priority (v0.3.0)

### Features
- [ ] **Persistent scan queue**
  - Database-backed queue for scan jobs
  - Prevent job loss on restart
  - Priority queue (critical images first)
  - Configurable workers
  - Files: `internal/scanner/scanner.go:72-78`

- [ ] **Image replication**
  - Push to multiple registries
  - Pull from upstream registries
  - Configurable replication rules
  - Support for Docker Hub, ECR, GCR

- [ ] **Storage backends**
  - Amazon S3 support
  - MinIO support
  - Azure Blob Storage support
  - Google Cloud Storage support

- [ ] **Advanced policy features**
  - Time-based policies (deny after sunset)
  - Quota enforcement (storage limits per namespace)
  - Image retention policies (auto-delete old images)
  - Compliance policies (require certain labels)

### User Experience
- [ ] **Web Dashboard (React)**
  - Repository browser
  - Image list with tags
  - Vulnerability reports
  - Policy configuration UI
  - User management
  - Audit log viewer

- [ ] **CLI improvements**
  - `ads-registry list repos` - List all repositories
  - `ads-registry list tags <repo>` - List tags
  - `ads-registry scan <repo:tag>` - Trigger manual scan
  - `ads-registry gc` - Run garbage collection
  - `ads-registry policy add <expr>` - Add policy rule

- [ ] **Image signing workflow**
  - Integrated Cosign support
  - Automatic signing on push
  - Signature verification enforcement
  - Key management

## Low Priority (v0.4.0+)

### Advanced Features
- [ ] **Multi-region support**
  - Geographic replication
  - Closest-region routing
  - Latency-based load balancing

- [ ] **Image mirroring**
  - Cache images from Docker Hub
  - Cache images from other registries
  - Reduce external bandwidth

- [ ] **Advanced scanning**
  - SBOM generation
  - License scanning
  - Secret detection
  - Malware scanning

- [ ] **Notification system**
  - Email notifications for vulnerabilities
  - Slack integration
  - Microsoft Teams integration
  - Custom webhook templates

### Performance
- [ ] **Blob deduplication across namespaces**
  - Reference counting
  - Shared blob storage
  - Significant storage savings

- [ ] **CDN integration**
  - CloudFront support
  - Fastly support
  - Cache blob layers globally

- [ ] **Compression**
  - Gzip compression for blobs
  - Zstd compression support
  - Configurable compression levels

## Bug Fixes

### Known Issues
- [ ] Fix: Scanner worker not stopped on graceful shutdown
- [ ] Fix: Webhook dispatcher blocks on slow endpoints
- [ ] Fix: No limit on number of tags per repository
- [ ] Fix: Race condition in policy cache invalidation
- [ ] Fix: Manifest list support incomplete
- [ ] Fix: No validation of media types

### Edge Cases
- [ ] Handle: Very large manifests (>10MB)
- [ ] Handle: Concurrent deletes of same blob
- [ ] Handle: Disk space exhaustion
- [ ] Handle: Database connection pool exhaustion
- [ ] Handle: Network timeouts during upload

## Documentation

### Missing Docs
- [ ] API reference documentation (OpenAPI/Swagger)
- [ ] Policy language guide (CEL examples)
- [ ] Starlark automation guide
- [ ] Troubleshooting guide
- [ ] Performance tuning guide
- [ ] Security best practices
- [ ] Backup and recovery guide
- [ ] Migration guide from other registries

### Tutorials
- [ ] Tutorial: Setting up for production
- [ ] Tutorial: Kubernetes deployment
- [ ] Tutorial: Multi-region setup
- [ ] Tutorial: Custom policy rules
- [ ] Tutorial: Vulnerability scanning workflow
- [ ] Tutorial: CI/CD integration

## Infrastructure

### DevOps
- [ ] **CI/CD Pipeline**
  - Automated builds on commit
  - Run tests on PR
  - Security scanning of code
  - Docker image publishing
  - GitHub releases

- [ ] **Terraform modules**
  - AWS deployment
  - GCP deployment
  - Azure deployment
  - On-premises deployment

- [ ] **Helm chart**
  - Kubernetes deployment
  - Configurable values
  - HA configuration
  - Monitoring integration

## Technical Debt

### Code Quality
- [ ] Refactor: Extract constants to config (magic numbers)
- [ ] Refactor: Split router.go into smaller files
- [ ] Refactor: Add interface for storage provider
- [ ] Refactor: Consistent error handling patterns
- [ ] Refactor: Remove duplicate code in sqlite/postgres

### Architecture
- [ ] Redesign: Event system for async operations
- [ ] Redesign: Plugin system for scanners
- [ ] Redesign: Middleware chain for policies
- [ ] Redesign: Cache abstraction layer

## Research & Investigation

### Future Exploration
- [ ] Investigate: WASM for policy execution
- [ ] Investigate: eBPF for network security
- [ ] Investigate: GPU acceleration for scanning
- [ ] Investigate: AI/ML for vulnerability prioritization
- [ ] Investigate: Blockchain for image provenance

---

## Contribution Guidelines

Want to help? Pick an item from this list!

1. Comment on the GitHub issue (or create one)
2. Fork the repository
3. Create a feature branch
4. Make your changes
5. Add tests
6. Submit a pull request

## Priority Definitions

- **Critical**: Blocks production use or causes data loss
- **High**: Significant impact on operations or security
- **Medium**: Improves user experience or performance
- **Low**: Nice to have, minimal impact

## Timeline

- **v0.2.0**: Target Q2 2026 (Critical + High priority items)
- **v0.3.0**: Target Q3 2026 (Medium priority features)
- **v0.4.0**: Target Q4 2026 (Advanced features)

## Completed ✅

Items moved to CHANGELOG.md when completed.
