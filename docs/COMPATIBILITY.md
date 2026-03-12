# ADS Container Registry - Compatibility System

## Overview

The ADS Registry Compatibility System provides production-grade workarounds for broken container registry clients, inspired by Postfix's pragmatic approach to handling broken mail clients. This system automatically detects client types and versions, then applies appropriate fixes to ensure compatibility.

## Why Do We Need This?

Container registry clients have bugs. Docker 29.2.0 has a manifest upload bug that causes "short copy: wrote 1 of 2858" errors. Podman sometimes sends malformed digests. Containerd expects specific header formats. Rather than rejecting these clients, we follow Postfix's philosophy:

> **Be liberal in what you accept, conservative in what you send.**

The compatibility system provides:

- Automatic client detection and version identification
- Configurable workarounds for known client bugs
- Comprehensive observability of which fixes are being applied
- Easy removal path when clients fix their bugs

## Quick Start

### Minimal Configuration

Add this to your `config.json`:

```json
{
  "compatibility": {
    "enabled": true,
    "docker_client_workarounds": {
      "enable_docker_29_manifest_fix": true,
      "force_http1_for_manifests": true,
      "extra_flushes": 3
    },
    "observability": {
      "log_workarounds": true,
      "enable_metrics": true
    }
  }
}
```

This enables the Docker 29.2.0 manifest fix and basic observability.

### Verify It's Working

After starting the registry, check logs for:

```
[COMPAT] Client detected: docker/29.2.0 (protocol=docker, http=HTTP/2.0, ua=Docker/29.2.0 ...)
[COMPAT] Activating workarounds for docker/29.2.0: [docker_29_manifest_fix] (path=/v2/myorg/myapp/manifests/latest)
```

Check Prometheus metrics at `http://localhost:5005/metrics`:

```
ads_registry_compat_workaround_activations_total{client="docker",version="29.2.0",workaround="docker_29_manifest_fix"} 42
ads_registry_compat_client_detections_total{client="docker",version="29.2.0",protocol="docker"} 150
```

## Configuration Reference

### Docker Client Workarounds

Fixes for Docker-specific bugs.

```json
{
  "docker_client_workarounds": {
    "enable_docker_29_manifest_fix": true,
    "force_http1_for_manifests": true,
    "disable_chunked_encoding": false,
    "max_manifest_size": 10485760,
    "extra_flushes": 3,
    "header_write_delay_ms": 0
  }
}
```

**enable_docker_29_manifest_fix** (boolean, default: `true`)
- Activates workaround for Docker 29.x "short copy" manifest bug
- Applies extra flushes and buffering to manifest uploads
- Issue: https://github.com/docker/cli/issues/XXXX
- Removal target: Docker 30.0.0+

**force_http1_for_manifests** (boolean, default: `true`)
- Forces HTTP/1.1 for manifest operations
- Helps with HTTP/2 chunked transfer bugs in various Docker versions
- Minimal performance impact

**disable_chunked_encoding** (boolean, default: `false`)
- Disables Transfer-Encoding: chunked for responses
- Only enable if specific clients have chunked transfer issues

**max_manifest_size** (integer, default: `10485760` = 10MB)
- Maximum manifest size before applying buffering workaround
- Set to 0 to buffer all manifests

**extra_flushes** (integer, default: `3`)
- Number of additional Flush() calls after writing manifest response
- Docker 29.x needs 3+ flushes to avoid "short copy" errors

**header_write_delay_ms** (integer, default: `0`)
- Artificial delay after header write in milliseconds
- Only needed for extremely buggy clients
- Usually leave at 0

### Protocol Emulation

Controls protocol compatibility modes.

```json
{
  "protocol_emulation": {
    "emulate_docker_registry_v2": true,
    "emulate_distribution_v3": false,
    "expose_oci_features": true,
    "strict_mode": false
  }
}
```

**emulate_docker_registry_v2** (boolean, default: `true`)
- Emulate Docker Registry v2 API quirks for maximum compatibility
- Recommended: leave enabled unless testing pure OCI compliance

**emulate_distribution_v3** (boolean, default: `false`)
- Future-proofing for OCI Distribution Spec v3
- Currently experimental

**expose_oci_features** (boolean, default: `true`)
- Enable OCI-specific features (artifacts, referrers API)
- Safe to enable alongside Docker emulation

**strict_mode** (boolean, default: `false`)
- Disable all workarounds and enforce pure OCI spec compliance
- Use for testing/validation only

### Broken Client Hacks

Workarounds for specific client bugs.

```json
{
  "broken_client_hacks": {
    "podman_digest_workaround": true,
    "skopeo_layer_reuse": true,
    "crane_manifest_format": "auto",
    "containerd_content_length": true,
    "buildkit_parallel_upload": false,
    "nerdctl_missing_headers": true
  }
}
```

**podman_digest_workaround** (boolean, default: `true`)
- Accept both `sha256:xxx` and `sha256-xxx` digest formats
- Podman 3.x sometimes sends incorrect delimiter
- Issue: https://github.com/containers/podman/issues/XXXX

**skopeo_layer_reuse** (boolean, default: `true`)
- Always return 202 Accepted for cross-mount attempts
- Skopeo 1.x has layer reuse detection issues

**crane_manifest_format** (string: "auto", "docker", "oci", default: `"auto"`)
- Override manifest format returned to Crane (go-containerregistry)
- `"auto"` detects from Accept header (recommended)

**containerd_content_length** (boolean, default: `true`)
- Always set Content-Length header, even if 0
- Containerd 1.6.x doesn't handle missing Content-Length

**buildkit_parallel_upload** (boolean, default: `false`)
- Handle parallel uploads without proper coordination
- Only enable if seeing BuildKit upload conflicts

**nerdctl_missing_headers** (boolean, default: `true`)
- Add missing required headers for nerdctl clients
- Safe to enable

### TLS Compatibility

TLS and HTTP version negotiation.

```json
{
  "tls_compatibility": {
    "min_tls_version": "1.2",
    "enable_legacy_ciphers": false,
    "http2_enabled": true,
    "force_http1_for_clients": ["Docker/29\\..*"],
    "alpn_protocols": ["h2", "http/1.1"]
  }
}
```

**min_tls_version** (string: "1.0", "1.1", "1.2", "1.3", default: `"1.2"`)
- Minimum TLS version for connections
- Recommended: "1.2" or higher

**enable_legacy_ciphers** (boolean, default: `false`)
- Enable legacy cipher suites for old clients
- Security warning: Only enable if absolutely necessary

**http2_enabled** (boolean, default: `true`)
- Enable HTTP/2 support
- Disable if seeing widespread HTTP/2 issues

**force_http1_for_clients** (array of regex strings, default: `["Docker/29\\..*"]`)
- Force HTTP/1.1 for matching User-Agent patterns
- Example: `["Docker/29\\..*", "containerd/1\\.6\\..*"]`

**alpn_protocols** (array of strings, default: `["h2", "http/1.1"]`)
- ALPN protocol preferences
- Order matters: first match wins

### Header Workarounds

HTTP header compatibility fixes.

```json
{
  "header_workarounds": {
    "always_send_distribution_api_version": true,
    "content_type_fixups": true,
    "location_header_format": "absolute",
    "enable_cors": false,
    "accept_malformed_accept": true,
    "normalize_digest_header": true
  }
}
```

**always_send_distribution_api_version** (boolean, default: `true`)
- Always send `Docker-Distribution-API-Version: registry/2.0` header
- Required by old Docker clients

**content_type_fixups** (boolean, default: `true`)
- Normalize `application/vnd.docker.*` vs `application/vnd.oci.*`
- Converts based on client protocol preference

**location_header_format** (string: "absolute", "relative", default: `"absolute"`)
- Format for Location headers in redirects
- "absolute": full URL (recommended)
- "relative": path only

**enable_cors** (boolean, default: `false`)
- Add CORS headers for web-based clients
- Enable if using registry from browsers

**accept_malformed_accept** (boolean, default: `true`)
- Accept invalid Accept header formats
- Some clients send malformed media type lists

**normalize_digest_header** (boolean, default: `true`)
- Normalize Docker-Content-Digest header format
- Ensures consistent `sha256:` prefix

### Rate Limit Exceptions

Exempt trusted clients from rate limits.

```json
{
  "rate_limit_exceptions": {
    "trusted_registries": ["gcr.io", "docker.io"],
    "cicd_user_agents": ["GitLab-Runner/.*", "GitHub-Actions/.*"],
    "trusted_ip_ranges": ["10.0.0.0/8"],
    "bypass_for_authenticated": false
  }
}
```

**trusted_registries** (array of strings, default: `[]`)
- Registry hostnames to exempt from rate limits
- For cross-registry replication

**cicd_user_agents** (array of regex strings, default: `["GitLab-Runner/.*", "GitHub-Actions/.*"]`)
- CI/CD user agent patterns to exempt
- Prevents build failures due to rate limits

**trusted_ip_ranges** (array of CIDR strings, default: `[]`)
- IP ranges to exempt from rate limits
- Example: `["10.0.0.0/8", "192.168.0.0/16"]`

**bypass_for_authenticated** (boolean, default: `false`)
- Bypass rate limits for all authenticated users
- Security consideration: may allow abuse

### Observability

Logging and metrics configuration.

```json
{
  "observability": {
    "log_workarounds": true,
    "log_client_detection": true,
    "enable_metrics": true,
    "metrics_prefix": "ads_registry_compat_",
    "log_sample_rate": 1.0,
    "log_success_only": false
  }
}
```

**log_workarounds** (boolean, default: `true`)
- Log when workarounds are activated
- Recommended for troubleshooting

**log_client_detection** (boolean, default: `true`)
- Log detected client information
- Useful for understanding client distribution

**enable_metrics** (boolean, default: `true`)
- Export Prometheus metrics
- Recommended for production

**metrics_prefix** (string, default: `"ads_registry_compat_"`)
- Prefix for Prometheus metric names

**log_sample_rate** (float: 0.0 to 1.0, default: `1.0`)
- Sample rate for detailed logging
- 1.0 = log everything, 0.1 = log 10% of requests
- Reduce in high-traffic environments

**log_success_only** (boolean, default: `false`)
- Only log successful workaround applications
- Omit failures/errors from logs

## Metrics

The compatibility system exports these Prometheus metrics:

### ads_registry_compat_workaround_activations_total
Counter of workaround activations.

Labels:
- `client`: Client name (docker, podman, etc.)
- `version`: Client version
- `workaround`: Workaround name

Example:
```
ads_registry_compat_workaround_activations_total{client="docker",version="29.2.0",workaround="docker_29_manifest_fix"} 42
```

### ads_registry_compat_client_detections_total
Counter of client detections.

Labels:
- `client`: Client name
- `version`: Client version
- `protocol`: Protocol (docker, oci, containerd)

### ads_registry_compat_http_protocol_overrides_total
Counter of HTTP protocol overrides (HTTP/2 → HTTP/1.1).

Labels:
- `from`: Original protocol
- `to`: Override protocol
- `reason`: Reason for override

### ads_registry_compat_manifest_fixes_total
Counter of manifest-related fixes.

Labels:
- `client`: Client name
- `fix_type`: Type of fix (header_flush, extra_flush, etc.)

### ads_registry_compat_header_workarounds_total
Counter of header workarounds.

Labels:
- `header`: Header name
- `workaround`: Workaround type

### ads_registry_compat_workaround_duration_seconds
Histogram of workaround processing time.

Labels:
- `workaround`: Workaround name

## Troubleshooting

### Docker 29.2.0 "short copy" Error Still Occurs

**Symptom**: Still seeing "short copy: wrote 1 of 2858" errors

**Solution**:
1. Verify compatibility is enabled in config.json
2. Check logs for workaround activation:
   ```
   [COMPAT] Activating workarounds for docker/29.2.0: [docker_29_manifest_fix]
   ```
3. If not activating, verify User-Agent is detected:
   ```
   [COMPAT] Client detected: docker/29.2.0
   ```
4. Try increasing `extra_flushes` to 5:
   ```json
   {
     "docker_client_workarounds": {
       "extra_flushes": 5
     }
   }
   ```
5. Force HTTP/1.1 by ensuring:
   ```json
   {
     "docker_client_workarounds": {
       "force_http1_for_manifests": true
     }
   }
   ```

### High Memory Usage

**Symptom**: Registry consuming excessive memory

**Cause**: Manifest buffering for workarounds

**Solution**:
1. Reduce `max_manifest_size`:
   ```json
   {
     "docker_client_workarounds": {
       "max_manifest_size": 5242880
     }
   }
   ```
2. Check metrics for buffered manifests
3. Consider disabling buffering if not needed

### Unknown Client Not Detected

**Symptom**: Client not being detected, shows as "unknown"

**Solution**:
1. Check logs for User-Agent string
2. Add detection pattern (requires code change in `detection.go`)
3. File issue with User-Agent string for future inclusion

### Too Much Logging

**Symptom**: Log files growing too large with compatibility messages

**Solution**:
1. Reduce log sample rate:
   ```json
   {
     "observability": {
       "log_sample_rate": 0.1
     }
   }
   ```
2. Disable client detection logging:
   ```json
   {
     "observability": {
       "log_client_detection": false
     }
   }
   ```
3. Only log workarounds (not detections)

## Testing Client Compatibility

### Simulate Docker 29.2.0 Client

```bash
# Set User-Agent header to simulate Docker 29.2.0
curl -v -H "User-Agent: Docker/29.2.0" \
  http://localhost:5005/v2/
```

Check logs for detection:
```
[COMPAT] Client detected: docker/29.2.0 (protocol=docker, http=HTTP/1.1, ua=Docker/29.2.0)
```

### Test Manifest Upload with Workarounds

```bash
# Create a test manifest
cat > manifest.json <<EOF
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "digest": "sha256:abc123..."
  }
}
EOF

# Upload with Docker 29.2.0 simulation
curl -v -X PUT \
  -H "User-Agent: Docker/29.2.0" \
  -H "Content-Type: application/vnd.docker.distribution.manifest.v2+json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  --data-binary @manifest.json \
  http://localhost:5005/v2/myorg/myapp/manifests/latest
```

Check logs for workaround activation:
```
[COMPAT] Activating workarounds for docker/29.2.0: [docker_29_manifest_fix]
```

### Verify Metrics

```bash
# Check Prometheus metrics
curl http://localhost:5005/metrics | grep compat

# Expected output:
# ads_registry_compat_workaround_activations_total{client="docker",version="29.2.0",workaround="docker_29_manifest_fix"} 1
# ads_registry_compat_client_detections_total{client="docker",version="29.2.0",protocol="docker"} 2
```

## Best Practices

### Production Deployment

1. **Start with defaults**: The default configuration handles most cases
2. **Monitor metrics**: Watch workaround activation rates
3. **Log everything initially**: Set `log_sample_rate: 1.0` during rollout
4. **Reduce logging later**: Lower to 0.1 after validation
5. **Review regularly**: Check which workarounds are still needed

### Security Considerations

1. **Don't bypass auth**: Workarounds don't affect authentication
2. **Audit workarounds**: Log all activations for security review
3. **Update regularly**: Remove workarounds when clients are fixed
4. **Monitor abuse**: Watch for clients exploiting compatibility features

### Performance Optimization

1. **Sample logging**: Use `log_sample_rate` in high-traffic environments
2. **Disable unused workarounds**: Turn off what you don't need
3. **Monitor overhead**: Check `workaround_duration_seconds` metrics
4. **Benchmark**: Test performance impact before production

## Migration Guide

### From No Compatibility System

1. Add compatibility section to config.json (use minimal example)
2. Restart registry
3. Monitor logs for workaround activations
4. Adjust configuration based on observed client behavior

### Disabling Workarounds

When a client bug is fixed and no longer in use:

1. Identify the workaround from metrics (e.g., no activations in 30 days)
2. Disable in configuration:
   ```json
   {
     "docker_client_workarounds": {
       "enable_docker_29_manifest_fix": false
     }
   }
   ```
3. Monitor for errors after change
4. Remove configuration if no issues

## FAQ

**Q: Does the compatibility system affect performance?**
A: Minimal impact. Client detection adds ~10-20μs per request. Workarounds add ~100-500μs when activated. For most registries, this is negligible.

**Q: Can I disable the compatibility system entirely?**
A: Yes, set `"enabled": false` in the compatibility section. This bypasses all workarounds.

**Q: What happens if a client isn't detected?**
A: It's treated as "unknown" and gets standard OCI-compliant behavior (no workarounds).

**Q: How do I add support for a new client?**
A: File an issue with the User-Agent string. We'll add detection in the next release.

**Q: Is this similar to Postfix's compatibility system?**
A: Yes! We were directly inspired by Postfix's pragmatic approach to broken SMTP clients. Like Postfix, we document every hack and provide easy removal paths.

**Q: Can I use this with other OCI registries?**
A: The compatibility middleware is specific to ADS Registry, but the concepts are portable. The detection and workaround patterns could be adapted.

## References

- [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec)
- [Docker Registry HTTP API V2](https://docs.docker.com/registry/spec/api/)
- [Postfix SMTP Workarounds](http://www.postfix.org/COMPATIBILITY_README.html)
- [Architecture Document](../internal/compat/ARCHITECTURE.md)

## Support

For issues with the compatibility system:

1. Check this documentation
2. Review metrics and logs
3. File an issue with:
   - Client User-Agent string
   - Observed behavior
   - Expected behavior
   - Logs and metrics
