# ADS Container Registry - Compatibility System Documentation Index

## Quick Start (Choose Your Path)

### I Just Want to Fix Docker 29.2.0 Errors
→ **Start Here**: [Quick Reference - Docker 29.2.0 Fix](docs/COMPATIBILITY-QUICK-REFERENCE.md#docker-2920-short-copy-error)

30-second fix:
```json
{"compatibility": {"enabled": true}}
```

### I Want to Understand the System
→ **Start Here**: [Implementation Summary](COMPATIBILITY-SYSTEM-SUMMARY.md)

Then read: [Architecture Documentation](internal/compat/ARCHITECTURE.md)

### I'm Deploying to Production
→ **Start Here**: [Deployment Checklist](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md)

Then read: [Integration Guide](docs/COMPATIBILITY-INTEGRATION.md)

### I'm Writing Tests
→ **Start Here**: [Testing Guide](docs/COMPATIBILITY-TESTING.md)

### I Need Configuration Help
→ **Start Here**: [User Documentation](docs/COMPATIBILITY.md)

Then see: [Config Examples](config-examples/)

## Complete Documentation Map

### 1. Overview & Planning

| Document | Purpose | Audience | When to Read |
|----------|---------|----------|--------------|
| [COMPATIBILITY-INDEX.md](COMPATIBILITY-INDEX.md) | This file - navigation | Everyone | First |
| [COMPATIBILITY-SYSTEM-SUMMARY.md](COMPATIBILITY-SYSTEM-SUMMARY.md) | Executive summary | Tech leads, managers | Before implementation |
| [internal/compat/ARCHITECTURE.md](internal/compat/ARCHITECTURE.md) | Technical architecture | Developers | Before coding |

### 2. User Documentation

| Document | Purpose | Audience | When to Read |
|----------|---------|----------|--------------|
| [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) | Complete user guide (35KB) | Operators, DevOps | Before deployment |
| [docs/COMPATIBILITY-QUICK-REFERENCE.md](docs/COMPATIBILITY-QUICK-REFERENCE.md) | Quick fixes & patterns | On-call engineers | During incidents |
| [config-examples/compatibility-minimal.json](config-examples/compatibility-minimal.json) | Minimal config | Quick start | First deployment |
| [config-examples/compatibility-full.json](config-examples/compatibility-full.json) | Complete config reference | Advanced users | Customization |

### 3. Developer Documentation

| Document | Purpose | Audience | When to Read |
|----------|---------|----------|--------------|
| [internal/compat/README.md](internal/compat/README.md) | Package documentation | Developers | Before modifying |
| [docs/COMPATIBILITY-TESTING.md](docs/COMPATIBILITY-TESTING.md) | Testing strategies | QA, Developers | Before testing |
| [docs/COMPATIBILITY-INTEGRATION.md](docs/COMPATIBILITY-INTEGRATION.md) | Integration details | Developers | Before integrating |

### 4. Operations

| Document | Purpose | Audience | When to Read |
|----------|---------|----------|--------------|
| [COMPATIBILITY-DEPLOYMENT-CHECKLIST.md](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md) | Deployment steps | DevOps | Before deployment |
| [docs/COMPATIBILITY-QUICK-REFERENCE.md](docs/COMPATIBILITY-QUICK-REFERENCE.md) | Troubleshooting | On-call | During incidents |

## File Locations

### Implementation Code
```
/Users/ryan/development/ads-registry/internal/compat/
├── config.go           (7KB)  - Configuration structs
├── detection.go        (8KB)  - Client detection logic
├── middleware.go       (10KB) - HTTP middleware
├── metrics.go          (3KB)  - Prometheus metrics
├── ARCHITECTURE.md     (10KB) - Architecture docs
└── README.md           (8KB)  - Package docs
```

### Configuration
```
/Users/ryan/development/ads-registry/
├── internal/config/config.go         - Extended with CompatibilityConfig
└── cmd/ads-registry/cmd/serve.go     - Middleware integration

/Users/ryan/development/ads-registry/config-examples/
├── compatibility-full.json           - Complete reference
└── compatibility-minimal.json        - Quick start
```

### Documentation
```
/Users/ryan/development/ads-registry/docs/
├── COMPATIBILITY.md                  (35KB) - User guide
├── COMPATIBILITY-TESTING.md          (20KB) - Testing guide
├── COMPATIBILITY-INTEGRATION.md      (15KB) - Integration guide
└── COMPATIBILITY-QUICK-REFERENCE.md  (8KB)  - Quick reference

/Users/ryan/development/ads-registry/
├── COMPATIBILITY-SYSTEM-SUMMARY.md   (6KB)  - Implementation summary
├── COMPATIBILITY-DEPLOYMENT-CHECKLIST.md    - Deployment guide
└── COMPATIBILITY-INDEX.md (this file)       - Navigation
```

## Documentation by Role

### DevOps Engineer Deploying to Production

**Must Read**:
1. [Deployment Checklist](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md) - Step-by-step deployment
2. [User Documentation](docs/COMPATIBILITY.md) - Configuration reference
3. [Quick Reference](docs/COMPATIBILITY-QUICK-REFERENCE.md) - Troubleshooting

**Optional**:
- [Integration Guide](docs/COMPATIBILITY-INTEGRATION.md) - How it works
- [Implementation Summary](COMPATIBILITY-SYSTEM-SUMMARY.md) - Overview

### Developer Modifying the Code

**Must Read**:
1. [Architecture](internal/compat/ARCHITECTURE.md) - System design
2. [Package README](internal/compat/README.md) - Code organization
3. [Testing Guide](docs/COMPATIBILITY-TESTING.md) - How to test

**Optional**:
- [Integration Guide](docs/COMPATIBILITY-INTEGRATION.md) - Integration points
- [User Documentation](docs/COMPATIBILITY.md) - User-facing behavior

### QA Engineer Testing the Feature

**Must Read**:
1. [Testing Guide](docs/COMPATIBILITY-TESTING.md) - Testing strategies
2. [Quick Reference](docs/COMPATIBILITY-QUICK-REFERENCE.md) - Test scenarios
3. [User Documentation](docs/COMPATIBILITY.md) - Expected behavior

**Optional**:
- [Integration Guide](docs/COMPATIBILITY-INTEGRATION.md) - Request flow
- [Implementation Summary](COMPATIBILITY-SYSTEM-SUMMARY.md) - Overview

### On-Call Engineer Troubleshooting

**Must Read**:
1. [Quick Reference](docs/COMPATIBILITY-QUICK-REFERENCE.md) - Quick fixes
2. [User Documentation § Troubleshooting](docs/COMPATIBILITY.md#troubleshooting) - Common issues

**Optional**:
- [Integration Guide](docs/COMPATIBILITY-INTEGRATION.md) - How it works
- [Deployment Checklist](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md) - Rollback procedures

### Technical Lead / Manager

**Must Read**:
1. [Implementation Summary](COMPATIBILITY-SYSTEM-SUMMARY.md) - Executive overview
2. [Architecture](internal/compat/ARCHITECTURE.md) - Technical design

**Optional**:
- [User Documentation](docs/COMPATIBILITY.md) - Full details
- [Deployment Checklist](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md) - Deployment plan

## Common Workflows

### Workflow 1: Quick Deploy for Docker 29.2.0 Fix

1. Read: [Quick Reference - Docker 29.2.0 Fix](docs/COMPATIBILITY-QUICK-REFERENCE.md#docker-2920-short-copy-error)
2. Copy: [config-examples/compatibility-minimal.json](config-examples/compatibility-minimal.json)
3. Follow: [Deployment Checklist § Quick Start](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md#deployment-steps)
4. Verify: [Quick Reference § Diagnostics](docs/COMPATIBILITY-QUICK-REFERENCE.md#quick-diagnostics)

### Workflow 2: Full Production Deployment

1. Read: [Implementation Summary](COMPATIBILITY-SYSTEM-SUMMARY.md)
2. Read: [User Documentation](docs/COMPATIBILITY.md)
3. Review: [config-examples/compatibility-full.json](config-examples/compatibility-full.json)
4. Test: [Testing Guide](docs/COMPATIBILITY-TESTING.md)
5. Deploy: [Deployment Checklist](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md)
6. Monitor: [Quick Reference § Metrics](docs/COMPATIBILITY-QUICK-REFERENCE.md#key-metrics)

### Workflow 3: Adding New Workaround

1. Read: [Architecture § Adding New Workarounds](internal/compat/ARCHITECTURE.md)
2. Read: [Package README](internal/compat/README.md)
3. Modify: `internal/compat/config.go` - Add configuration
4. Modify: `internal/compat/middleware.go` - Implement workaround
5. Test: [Testing Guide § Unit Tests](docs/COMPATIBILITY-TESTING.md#unit-testing)
6. Document: Update [User Documentation](docs/COMPATIBILITY.md)
7. Deploy: [Deployment Checklist](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md)

### Workflow 4: Troubleshooting Production Issue

1. Check: [Quick Reference § Common Issues](docs/COMPATIBILITY-QUICK-REFERENCE.md#common-issues--quick-fixes)
2. Diagnose: [Quick Reference § Diagnostics](docs/COMPATIBILITY-QUICK-REFERENCE.md#quick-diagnostics)
3. Fix: Apply configuration change from [Quick Reference](docs/COMPATIBILITY-QUICK-REFERENCE.md)
4. Escalate: If needed, reference [User Documentation § Troubleshooting](docs/COMPATIBILITY.md#troubleshooting)
5. Rollback: If necessary, follow [Deployment Checklist § Rollback](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md#rollback-plan)

## Documentation Size Reference

| Document | Size | Reading Time | Detail Level |
|----------|------|--------------|--------------|
| Quick Reference | 8KB | 10 min | Reference |
| Implementation Summary | 6KB | 15 min | Overview |
| Deployment Checklist | 4KB | 20 min | Checklist |
| Package README | 8KB | 20 min | Medium |
| Architecture | 10KB | 30 min | High |
| Integration Guide | 15KB | 30 min | High |
| Testing Guide | 20KB | 45 min | High |
| User Documentation | 35KB | 60 min | Complete |

**Total Documentation**: ~106KB (~3 hours reading)

## Key Concepts Reference

### Client Detection
- **What**: Parsing User-Agent headers to identify client type and version
- **Where**: [detection.go](internal/compat/detection.go)
- **Docs**: [Architecture § Client Detection](internal/compat/ARCHITECTURE.md#1-client-detection-layer)

### Workaround Activation
- **What**: Applying fixes based on detected client
- **Where**: [middleware.go](internal/compat/middleware.go)
- **Docs**: [Architecture § Middleware](internal/compat/ARCHITECTURE.md#3-middleware-architecture)

### Configuration
- **What**: YAML/JSON settings for all workarounds
- **Where**: [config.go](internal/compat/config.go)
- **Docs**: [User Documentation § Configuration](docs/COMPATIBILITY.md#configuration-reference)

### Metrics
- **What**: Prometheus metrics for observability
- **Where**: [metrics.go](internal/compat/metrics.go)
- **Docs**: [User Documentation § Metrics](docs/COMPATIBILITY.md#metrics)

## Supported Client Reference

Quick reference for detected clients:

| Client | Version Detection | Primary Workaround | Config Setting |
|--------|------------------|-------------------|---------------|
| Docker 29.x | ✓ | Manifest fix | `enable_docker_29_manifest_fix` |
| Podman 3.x | ✓ | Digest normalization | `podman_digest_workaround` |
| Containerd 1.6.x | ✓ | Content-Length | `containerd_content_length` |
| Skopeo | ✓ | Layer reuse | `skopeo_layer_reuse` |
| Crane | ✓ | Manifest format | `crane_manifest_format` |
| Buildkit | ✓ | Parallel upload | `buildkit_parallel_upload` |
| Nerdctl | ✓ | Missing headers | `nerdctl_missing_headers` |

Full details: [User Documentation § Supported Clients](docs/COMPATIBILITY.md)

## Metrics Quick Reference

| Metric | Purpose | Query |
|--------|---------|-------|
| Client detections | Track client distribution | `ads_registry_compat_client_detections_total` |
| Workaround activations | Track fix usage | `ads_registry_compat_workaround_activations_total` |
| Performance | Monitor overhead | `ads_registry_compat_workaround_duration_seconds` |

Full details: [User Documentation § Metrics](docs/COMPATIBILITY.md#metrics)

## Configuration Quick Reference

| Use Case | Config File |
|----------|-------------|
| Quick start (Docker 29.2.0 fix) | [compatibility-minimal.json](config-examples/compatibility-minimal.json) |
| Production high-traffic | See [Quick Reference](docs/COMPATIBILITY-QUICK-REFERENCE.md#production-high-traffic) |
| All workarounds enabled | [compatibility-full.json](config-examples/compatibility-full.json) |
| Strict OCI compliance | See [Quick Reference](docs/COMPATIBILITY-QUICK-REFERENCE.md#strict-oci-compliance-testing-only) |

Full details: [User Documentation § Configuration Reference](docs/COMPATIBILITY.md#configuration-reference)

## Status & Version

- **Implementation Status**: ✅ Complete
- **Version**: 1.0
- **Implementation Date**: 2026-03-11
- **Build Status**: ✅ Passing
- **Documentation**: ✅ Complete
- **Production Ready**: ✅ Yes

## Getting Help

1. **Quick fixes**: [Quick Reference](docs/COMPATIBILITY-QUICK-REFERENCE.md)
2. **Common issues**: [User Documentation § Troubleshooting](docs/COMPATIBILITY.md#troubleshooting)
3. **Testing help**: [Testing Guide](docs/COMPATIBILITY-TESTING.md)
4. **Deployment help**: [Deployment Checklist](COMPATIBILITY-DEPLOYMENT-CHECKLIST.md)

## Contributing

To contribute:
1. Read: [Architecture](internal/compat/ARCHITECTURE.md)
2. Read: [Package README § Contributing](internal/compat/README.md#contributing)
3. Follow: [Testing Guide](docs/COMPATIBILITY-TESTING.md)
4. Update: All relevant documentation

## License

Same as parent ADS Container Registry project.

---

**Last Updated**: 2026-03-11
**Maintainer**: ADS Registry Team
**Documentation Version**: 1.0
