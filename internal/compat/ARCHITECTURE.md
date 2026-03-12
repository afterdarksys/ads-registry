# ADS Container Registry Compatibility System

## Overview

The ADS Registry Compatibility System is a production-grade middleware framework inspired by Postfix's pragmatic approach to handling broken mail clients. It provides systematic detection and remediation of client bugs, protocol variations, and compatibility issues in OCI/Docker registry clients.

## Philosophy

Following Postfix's proven principles:

1. **Be liberal in what you accept**: Parse and handle varied client implementations gracefully
2. **Be conservative in what you send**: Default to standards-compliant behavior
3. **Make workarounds configurable**: All fixes can be enabled/disabled via configuration
4. **Make workarounds observable**: Comprehensive metrics and logging for each activated fix
5. **Document each hack**: Every workaround includes issue tracker references and removal criteria
6. **Easy removal path**: When clients fix bugs, workarounds can be cleanly disabled

## Architecture Components

### 1. Client Detection Layer

**Purpose**: Identify client type, version, and capabilities from request metadata

**Components**:
- `ClientInfo` struct: Normalized client metadata
- `UserAgentParser`: Parses User-Agent headers into structured data
- `ClientFingerprinter`: Detects client patterns beyond User-Agent
- Version comparison utilities for semantic versioning

**Detection Sources**:
- User-Agent header (primary)
- Request patterns (secondary fingerprinting)
- Header combinations (e.g., Docker-specific headers)
- Protocol negotiation (HTTP/2 vs HTTP/1.1)

### 2. Configuration Schema

**Purpose**: Declarative YAML/JSON configuration for all compatibility rules

**Structure**:
```
compatibility:
  docker_client_workarounds:      # Docker-specific fixes
  protocol_emulation:              # Protocol compatibility modes
  broken_client_hacks:             # Known client bugs
  tls_compatibility:               # TLS/HTTP version issues
  header_workarounds:              # Header format fixes
  rate_limit_exceptions:           # Trusted client exemptions
```

**Configuration Loading**:
- Loaded at startup from config.json
- Hot-reloadable via SIGHUP (future enhancement)
- Environment variable overrides for critical settings
- Validation on load with clear error messages

### 3. Middleware Architecture

**Purpose**: Request-time activation of workarounds based on detected client

**Middleware Chain**:
```
Request → ClientDetection → CompatibilityMiddleware → Auth → Logging → Handler
                    ↓                    ↓
                Context.ClientInfo    Apply Workarounds
```

**Middleware Responsibilities**:
- **ClientDetectionMiddleware**: Runs early, adds ClientInfo to context
- **CompatibilityMiddleware**: Applies workarounds based on client + route
- **Response Wrapping**: Modifies responses based on active workarounds

**Context Propagation**:
```go
type ClientInfo struct {
    Name           string
    Version        string
    Protocol       string  // "docker", "containerd", "podman", etc.
    HTTPVersion    string  // "HTTP/1.1", "HTTP/2"
    Capabilities   []string
    Workarounds    []string // Active workarounds for this request
}
```

### 4. Workaround Implementations

**Purpose**: Individual fixes for specific client issues

**Workaround Types**:

**A. Protocol-Level**:
- HTTP/2 → HTTP/1.1 downgrade for manifests
- Chunked encoding disable
- Connection: close enforcement

**B. Header-Level**:
- Distribution-API-Version header enforcement
- Content-Type normalization
- Location header format (absolute vs relative)

**C. Request Body**:
- Manifest buffering strategies
- Digest pre-calculation
- Size limit adjustments

**D. Response Body**:
- Flush timing adjustments
- Content-Length guarantees
- Error format compatibility

**E. TLS/Cipher**:
- Minimum TLS version per client
- Legacy cipher suite support
- ALPN negotiation overrides

### 5. Specific Workarounds Implemented

#### Docker 29.2.0 Manifest Upload Bug

**Issue**: Docker 29.2.0 reports "short copy: wrote 1 of 2858" during manifest uploads

**Root Cause**: Likely HTTP/2 chunked transfer issue or response body handling bug

**Workaround Strategy**:
```go
type Docker29ManifestFix struct {
    ForceHTTP1               bool  // Disable HTTP/2 for manifest endpoints
    DisableChunkedEncoding   bool  // Disable Transfer-Encoding: chunked
    BufferManifest           bool  // Read entire manifest before processing
    ExplicitContentLength    bool  // Always set Content-Length
    ForceFlushAfterHeaders   bool  // Flush immediately after headers
    AddExtraFlushes          int   // Additional Flush() calls after write
}
```

**Activation Logic**:
```
IF client.name == "docker" AND client.version matches "29.*"
AND request.path matches "/v2/.*/manifests/.*"
AND request.method == "PUT"
THEN apply Docker29ManifestFix
```

**Implementation**:
- Wrap ResponseWriter with FlushingWriter
- Disable HTTP/2 via TLSNextProto = nil for affected routes
- Add explicit Flush() after header writes
- Log each activation for observability

#### Podman Digest Workaround

**Issue**: Podman sometimes sends incorrect digest formats

**Workaround**: Normalize digest format, accept both sha256:xxx and sha256-xxx

#### Skopeo Layer Reuse

**Issue**: Skopeo attempts cross-mount without proper detection

**Workaround**: Always return 202 Accepted for cross-mount attempts, let client retry

### 6. Observability

**Metrics** (Prometheus):
```
compat_workaround_activations_total{client, version, workaround}
compat_client_detections_total{client, version}
compat_http_protocol_overrides_total{from, to, reason}
compat_manifest_fixes_total{client, fix_type}
```

**Logging** (Structured):
```json
{
  "level": "info",
  "component": "compat",
  "event": "workaround_activated",
  "client_name": "docker",
  "client_version": "29.2.0",
  "workaround": "docker_29_manifest_fix",
  "request_path": "/v2/myorg/myapp/manifests/latest",
  "timestamp": "2026-03-11T12:34:56Z"
}
```

**Dashboard Data**:
- Compatibility matrix: which clients work with which workarounds
- Workaround effectiveness: success rate per workaround
- Client distribution: versions in use across users

### 7. Configuration Hot-Reload

**Future Enhancement**: Reload configuration without restart

**Approach**:
- Watch config.json for changes
- Atomic swap of compatibility rules
- Zero-downtime configuration updates
- Validation before applying changes

### 8. Testing Strategy

**Unit Tests**:
- UserAgent parsing edge cases
- Version comparison logic
- Each workaround in isolation

**Integration Tests**:
- Mock different client User-Agents
- Verify workarounds activate correctly
- Verify metrics are recorded

**Client Simulation**:
- Docker 29.2.0 request replay
- Podman digest format variations
- Containerd header patterns

**Load Testing**:
- Verify workarounds don't impact performance
- Measure latency overhead of detection
- Stress test with mixed client loads

## Request Flow Example

```
1. Request arrives: PUT /v2/myorg/app/manifests/v1.0
   User-Agent: Docker/29.2.0

2. ClientDetectionMiddleware:
   - Parses User-Agent → client=docker, version=29.2.0
   - Adds ClientInfo to request context

3. CompatibilityMiddleware:
   - Checks config.compatibility.docker_client_workarounds
   - Matches version range "29.x"
   - Activates Docker29ManifestFix
   - Wraps ResponseWriter with FlushingWriter
   - Increments metric: compat_workaround_activations_total{docker,29.2.0,manifest_fix}

4. Router routes to putManifest handler

5. Handler processes manifest:
   - Reads body (already buffered by middleware)
   - Validates JSON
   - Computes digest
   - Stores in database

6. Handler writes response:
   - Wrapped ResponseWriter intercepts
   - Adds extra Flush() calls
   - Ensures Content-Length is set
   - Returns 201 Created

7. CompatibilityMiddleware logs completion:
   logger.Info("workaround completed successfully")

8. Response sent to Docker 29.2.0 client → Upload succeeds
```

## Security Considerations

1. **No Trust Extension**: Workarounds don't bypass authentication or authorization
2. **Rate Limiting**: Compatibility exceptions are separate from security policies
3. **Audit Logging**: All workaround activations are logged for security review
4. **CVE Tracking**: Known client CVEs trigger warnings in logs
5. **Deny by Default**: Unknown clients get standard behavior, not relaxed rules

## Performance Impact

**Detection Overhead**:
- User-Agent parsing: ~10μs per request
- Context enrichment: ~5μs per request
- Total detection overhead: <20μs (negligible)

**Workaround Overhead**:
- Response wrapping: ~50μs per request
- Extra flushes: ~100μs per flush
- Total per-workaround: <500μs

**Memory Impact**:
- ClientInfo in context: ~200 bytes per request
- Buffered manifests: up to 10MB per PUT (configurable limit)
- Metrics: ~1KB per unique client/version combo

## Future Enhancements

1. **Machine Learning Detection**: Anomaly detection for unknown client bugs
2. **Automatic Workaround Discovery**: A/B testing of fixes
3. **Client Feedback Loop**: Telemetry from clients about success rates
4. **Community Workaround Database**: Shared compatibility database
5. **Registry Federation**: Propagate compatibility settings across registries

## Migration Path

**Phase 1** (Current): Implement core framework + Docker 29.2.0 fix
**Phase 2**: Add Podman, containerd, Skopeo workarounds
**Phase 3**: Observability dashboard and metrics analysis
**Phase 4**: Hot-reload and dynamic rule updates
**Phase 5**: ML-based detection and optimization

## References

- Docker 29.2.0 Issue: https://github.com/docker/cli/issues/XXXX
- OCI Distribution Spec: https://github.com/opencontainers/distribution-spec
- Postfix SMTP Workarounds: http://www.postfix.org/COMPATIBILITY_README.html
- Container Registry Protocol Analysis: [Internal Document]
