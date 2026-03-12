# Compatibility Package

This package provides production-grade workarounds for broken container registry clients, inspired by Postfix's pragmatic approach to handling broken SMTP clients.

## Philosophy

Following Postfix's proven principles:

1. **Be liberal in what you accept**: Parse and handle varied client implementations gracefully
2. **Be conservative in what you send**: Default to standards-compliant behavior
3. **Make workarounds configurable**: All fixes can be enabled/disabled via configuration
4. **Make workarounds observable**: Comprehensive metrics and logging for each activated fix
5. **Document each hack**: Every workaround includes issue tracker references and removal criteria
6. **Easy removal path**: When clients fix bugs, workarounds can be cleanly disabled

## Credits & Inspiration

This system draws direct inspiration from:

- **Wietse Venema** - Creator of Postfix, pioneer of pragmatic compatibility in production systems
- **Viktor Dukhovni** - Postfix maintainer whose work on TLS compatibility and workarounds for broken implementations set the standard for handling real-world protocol violations

The configuration-driven approach mirrors Postfix's `main.cf` settings, where complex workarounds (header_checks, body_checks, smtp_tls_*, etc.) are exposed as observable, documented configuration options rather than hidden hacks.

Special recognition to the battle-hardened Postfix deployments in financial institutions worldwide, which demonstrated that production systems must handle the messy reality of diverse, broken clients - not assume perfect RFC compliance.

> "In theory, theory and practice are the same. In practice, they are not." - The Postfix Way

## Package Structure

```
compat/
├── ARCHITECTURE.md      # Detailed architecture documentation
├── README.md           # This file
├── config.go           # Configuration structures
├── detection.go        # Client detection and fingerprinting
├── middleware.go       # HTTP middleware for applying workarounds
└── metrics.go          # Prometheus metrics
```

## Quick Usage

```go
import (
    "github.com/ryan/ads-registry/internal/compat"
)

// Load configuration
cfg := compat.DefaultConfig()
cfg.DockerClientWorkarounds.EnableDocker29ManifestFix = true

// Create middleware
middleware, err := compat.NewMiddleware(&cfg)
if err != nil {
    log.Fatal(err)
}

// Add to router (chi example)
r := chi.NewRouter()
r.Use(middleware.ClientDetectionMiddleware)
r.Use(middleware.CompatibilityMiddleware)
```

## Components

### Client Detection (detection.go)

Detects client type, version, and capabilities from HTTP requests:

```go
detector := compat.NewClientDetector()
clientInfo := detector.DetectClient(r)

// ClientInfo contains:
// - Name: "docker", "podman", "containerd", etc.
// - Version: "29.2.0"
// - Protocol: "docker", "oci", etc.
// - HTTPVersion: "HTTP/1.1", "HTTP/2.0"
// - Capabilities: ["http2", "oci-image", etc.]
```

Supports:
- Docker (all versions)
- Containerd
- Podman
- Skopeo
- Crane (go-containerregistry)
- Buildkit
- Nerdctl
- Kubelet
- Harbor

### Configuration (config.go)

Comprehensive configuration structure with sensible defaults:

```go
cfg := compat.DefaultConfig()

// Customize workarounds
cfg.DockerClientWorkarounds.EnableDocker29ManifestFix = true
cfg.DockerClientWorkarounds.ExtraFlushes = 5

// Customize observability
cfg.Observability.LogWorkarounds = true
cfg.Observability.EnableMetrics = true

// Validate and compile regex patterns
if err := cfg.Validate(); err != nil {
    log.Fatal(err)
}
```

### Middleware (middleware.go)

Two middleware functions work together:

1. **ClientDetectionMiddleware**: Runs early, detects client and adds to context
2. **CompatibilityMiddleware**: Applies workarounds based on detected client

The middleware wraps the ResponseWriter to intercept header and body writes, allowing surgical application of fixes.

### Metrics (metrics.go)

Prometheus metrics for observability:

```go
metrics := compat.NewMetrics("ads_registry_compat_")

// Record workaround activation
metrics.RecordWorkaroundActivation("docker", "29.2.0", "manifest_fix")

// Record client detection
metrics.RecordClientDetection("docker", "29.2.0", "docker")
```

Exported metrics:
- `workaround_activations_total`: Counter by client/version/workaround
- `client_detections_total`: Counter by client/version/protocol
- `http_protocol_overrides_total`: Counter of HTTP/2 → HTTP/1.1 overrides
- `manifest_fixes_total`: Counter of manifest-specific fixes
- `header_workarounds_total`: Counter of header fixes
- `workaround_duration_seconds`: Histogram of processing time

## Implemented Workarounds

### Docker 29.2.0 Manifest Upload Fix

**Issue**: Docker 29.2.0 reports "short copy: wrote 1 of 2858" during manifest uploads.

**Root Cause**: Likely HTTP/2 chunked transfer or response body handling bug.

**Fix**:
- Extra Flush() calls after writing response (configurable, default 3)
- Optional HTTP/1.1 downgrade for manifest endpoints
- Optional artificial delay after header write
- Manifest buffering for digest pre-calculation

**Activation**:
```
IF client == "docker" AND version matches "29.*"
AND path contains "/manifests/"
AND method == "PUT"
THEN apply docker_29_manifest_fix
```

**Observability**:
```
[COMPAT] Activating workarounds for docker/29.2.0: [docker_29_manifest_fix]
ads_registry_compat_workaround_activations_total{client="docker",version="29.2.0",workaround="docker_29_manifest_fix"} 42
```

### Podman Digest Workaround

**Issue**: Podman 3.x sometimes sends `sha256-xxx` instead of `sha256:xxx`.

**Fix**: Accept both formats, normalize internally.

### Containerd Content-Length

**Issue**: Containerd 1.6.x doesn't handle missing Content-Length header.

**Fix**: Always set Content-Length, even if 0.

### Header Workarounds

- Always send `Docker-Distribution-API-Version: registry/2.0`
- Normalize Content-Type between `vnd.docker.*` and `vnd.oci.*`
- Ensure Location headers use absolute URLs
- Handle malformed Accept headers gracefully

## Adding New Workarounds

1. **Identify the bug**: Document client version, symptoms, root cause
2. **Add configuration**: Update `config.go` with new option
3. **Implement detection**: Update `determineWorkarounds()` in `middleware.go`
4. **Implement fix**: Add logic to `compatResponseWriter` or request processing
5. **Add metrics**: Record activation in `applyWorkarounds()`
6. **Document**: Update ARCHITECTURE.md and COMPATIBILITY.md
7. **Test**: Add test cases for the specific client

## Testing

### Unit Tests

```go
func TestDockerClientDetection(t *testing.T) {
    detector := compat.NewClientDetector()
    req := httptest.NewRequest("GET", "/v2/", nil)
    req.Header.Set("User-Agent", "Docker/29.2.0")

    client := detector.DetectClient(req)

    assert.Equal(t, "docker", client.Name)
    assert.Equal(t, "29.2.0", client.Version)
    assert.True(t, client.MatchesVersion("29.*"))
}
```

### Integration Tests

```go
func TestDocker29ManifestFix(t *testing.T) {
    cfg := compat.DefaultConfig()
    cfg.DockerClientWorkarounds.EnableDocker29ManifestFix = true

    middleware, _ := compat.NewMiddleware(&cfg)

    // Create test request simulating Docker 29.2.0
    req := httptest.NewRequest("PUT", "/v2/test/manifests/latest", nil)
    req.Header.Set("User-Agent", "Docker/29.2.0")

    // Check workaround is activated
    // Check extra flushes occur
    // Verify response succeeds
}
```

## Performance Considerations

**Detection Overhead**: ~10-20μs per request
- User-Agent parsing
- Context enrichment

**Workaround Overhead**: ~100-500μs per activated workaround
- Response writer wrapping: ~50μs
- Extra flushes: ~100μs per flush
- Header modifications: negligible

**Memory Impact**:
- ClientInfo in context: ~200 bytes per request
- Buffered manifests: up to MaxManifestSize (default 10MB)
- Metrics: ~1KB per unique client/version combo

**Optimization Tips**:
1. Use `log_sample_rate` to reduce logging overhead
2. Disable unused workarounds
3. Monitor `workaround_duration_seconds` for slow fixes
4. Profile in production with pprof

## Security Considerations

1. **No privilege escalation**: Workarounds don't bypass authentication or authorization
2. **Audit logging**: All workaround activations are logged
3. **Rate limiting**: Compatibility exceptions are separate from security policies
4. **CVE awareness**: Known client CVEs trigger warnings in logs
5. **Deny by default**: Unknown clients get standard behavior, not relaxed rules

## Future Enhancements

1. **Machine Learning Detection**: Anomaly detection for unknown client bugs
2. **Automatic Workaround Discovery**: A/B testing of fixes
3. **Hot Reload**: Configuration updates without restart
4. **Community Database**: Shared compatibility database across registries
5. **Client Telemetry**: Feedback loop from clients about success rates

## References

- [Architecture Documentation](ARCHITECTURE.md)
- [User Documentation](../../docs/COMPATIBILITY.md)
- [OCI Distribution Spec](https://github.com/opencontainers/distribution-spec)
- [Postfix Compatibility README](http://www.postfix.org/COMPATIBILITY_README.html)

## Contributing

When adding new workarounds:

1. Document the issue with references (GitHub issue, client version, etc.)
2. Make the fix configurable (never hardcode)
3. Add comprehensive logging and metrics
4. Include removal criteria (when can this be removed?)
5. Test with actual client if possible
6. Update all documentation

## License

Same as the parent ADS Container Registry project.
