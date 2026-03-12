# Compatibility System - Quick Reference Card

## 30-Second Setup

Add to `config.json`:
```json
{
  "compatibility": {
    "enabled": true
  }
}
```

Restart registry. Done.

## Common Issues & Quick Fixes

### Docker 29.2.0 "short copy" Error

**Symptom**: `Error response from daemon: failed to push manifest: short copy: wrote 1 of 2858`

**Quick Fix**:
```json
{
  "compatibility": {
    "enabled": true,
    "docker_client_workarounds": {
      "enable_docker_29_manifest_fix": true,
      "extra_flushes": 5
    }
  }
}
```

### Podman Digest Errors

**Symptom**: `MANIFEST_UNKNOWN` or digest format errors with Podman

**Quick Fix**:
```json
{
  "compatibility": {
    "enabled": true,
    "broken_client_hacks": {
      "podman_digest_workaround": true
    }
  }
}
```

### Containerd Missing Headers

**Symptom**: Containerd clients fail with header errors

**Quick Fix**:
```json
{
  "compatibility": {
    "enabled": true,
    "broken_client_hacks": {
      "containerd_content_length": true
    }
  }
}
```

## Quick Diagnostics

### Check if enabled
```bash
curl http://localhost:5005/metrics | grep compat_client_detections_total
```
If you see output, it's working.

### Check workaround activations
```bash
curl http://localhost:5005/metrics | grep workaround_activations_total
```

### View detected clients
```bash
# Check logs for:
[COMPAT] Client detected: docker/29.2.0
```

### Test with curl
```bash
curl -v -H "User-Agent: Docker/29.2.0" http://localhost:5005/v2/
# Should see Docker-Distribution-API-Version header
```

## Configuration Snippets

### Minimal (recommended starting point)
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

### Production High-Traffic
```json
{
  "compatibility": {
    "enabled": true,
    "docker_client_workarounds": {
      "enable_docker_29_manifest_fix": true,
      "extra_flushes": 3
    },
    "observability": {
      "log_workarounds": false,
      "enable_metrics": true,
      "log_sample_rate": 0.01
    }
  }
}
```

### All Workarounds Enabled
```json
{
  "compatibility": {
    "enabled": true,
    "docker_client_workarounds": {
      "enable_docker_29_manifest_fix": true,
      "force_http1_for_manifests": true,
      "extra_flushes": 3
    },
    "broken_client_hacks": {
      "podman_digest_workaround": true,
      "skopeo_layer_reuse": true,
      "containerd_content_length": true,
      "nerdctl_missing_headers": true
    },
    "header_workarounds": {
      "always_send_distribution_api_version": true,
      "content_type_fixups": true,
      "normalize_digest_header": true
    },
    "observability": {
      "log_workarounds": true,
      "enable_metrics": true
    }
  }
}
```

### Strict OCI Compliance (testing only)
```json
{
  "compatibility": {
    "enabled": true,
    "protocol_emulation": {
      "strict_mode": true
    }
  }
}
```

## Key Metrics

```promql
# Total clients detected
sum(ads_registry_compat_client_detections_total)

# Workaround activation rate
rate(ads_registry_compat_workaround_activations_total[5m])

# Client version distribution
sum by (client, version) (ads_registry_compat_client_detections_total)

# Performance impact (p99)
histogram_quantile(0.99, rate(ads_registry_compat_workaround_duration_seconds_bucket[5m]))
```

## Supported Clients

| Client | Detection | Workarounds Available |
|--------|-----------|---------------------|
| Docker | ✓ | Manifest fix, HTTP/1.1 force |
| Podman | ✓ | Digest normalization |
| Containerd | ✓ | Content-Length fix |
| Skopeo | ✓ | Layer reuse |
| Crane | ✓ | Manifest format |
| Buildkit | ✓ | Parallel upload |
| Nerdctl | ✓ | Missing headers |
| Kubelet | ✓ | Standard headers |
| Harbor | ✓ | Standard headers |

## Workaround Decision Matrix

| Client | Version | Issue | Workaround | Config Setting |
|--------|---------|-------|-----------|---------------|
| Docker | 29.x | Short copy | Extra flushes + HTTP/1.1 | `enable_docker_29_manifest_fix: true` |
| Podman | 3.x | Bad digest format | Accept both formats | `podman_digest_workaround: true` |
| Containerd | 1.6.x | Missing Content-Length | Force header | `containerd_content_length: true` |
| Skopeo | 1.x | Layer reuse | Accept cross-mount | `skopeo_layer_reuse: true` |

## Troubleshooting Flowchart

```
Is compatibility enabled?
    NO → Add "compatibility": {"enabled": true} to config.json
    YES ↓

Can you see client detections in metrics?
    NO → Check User-Agent header, verify middleware loaded
    YES ↓

Are workarounds activating?
    NO → Check client version matches patterns, enable specific workarounds
    YES ↓

Are errors still occurring?
    NO → Success! Monitor metrics.
    YES → Increase extra_flushes, check logs, file issue
```

## Performance Expectations

| Metric | Value | Impact |
|--------|-------|--------|
| Detection overhead | < 20μs | Negligible |
| Workaround overhead | < 500μs | < 0.05% |
| Memory per request | ~200 bytes | Negligible |
| Buffered manifest | Up to 10MB | Configurable |

## File Locations

| Purpose | Path |
|---------|------|
| Implementation | `/Users/ryan/development/ads-registry/internal/compat/` |
| Config schema | `/Users/ryan/development/ads-registry/internal/config/config.go` |
| Integration | `/Users/ryan/development/ads-registry/cmd/ads-registry/cmd/serve.go` |
| Full docs | `/Users/ryan/development/ads-registry/docs/COMPATIBILITY.md` |
| Examples | `/Users/ryan/development/ads-registry/config-examples/` |

## Quick Commands

```bash
# View all compatibility metrics
curl -s http://localhost:5005/metrics | grep compat

# Count workaround activations
curl -s http://localhost:5005/metrics | grep workaround_activations_total

# See client distribution
curl -s http://localhost:5005/metrics | grep client_detections_total

# Test with specific client
curl -H "User-Agent: Docker/29.2.0" http://localhost:5005/v2/

# Watch logs for activations
tail -f /var/log/ads-registry.log | grep COMPAT

# Monitor in real-time
watch -n 1 'curl -s http://localhost:5005/metrics | grep compat'
```

## Common Patterns

### Pattern 1: Unknown Client

```
[COMPAT] Client detected: unknown (protocol=unknown, http=HTTP/1.1, ua=CustomClient/1.0)
```

**Action**: Client gets standard OCI behavior. If issues occur, file issue with User-Agent.

### Pattern 2: Workaround Applied

```
[COMPAT] Client detected: docker/29.2.0 (protocol=docker, http=HTTP/2.0)
[COMPAT] Activating workarounds for docker/29.2.0: [docker_29_manifest_fix]
```

**Action**: Working as intended. Monitor success rate.

### Pattern 3: No Workarounds Needed

```
[COMPAT] Client detected: docker/30.0.0 (protocol=docker, http=HTTP/2.0)
```

**Action**: Client doesn't match any workaround patterns. Standard behavior.

## Default Values

| Setting | Default | Notes |
|---------|---------|-------|
| `enabled` | `true` | System enabled by default |
| `enable_docker_29_manifest_fix` | `false` | Must explicitly enable |
| `force_http1_for_manifests` | `false` | Must explicitly enable |
| `extra_flushes` | `3` | Increase if needed |
| `max_manifest_size` | `10485760` (10MB) | Adjust for large manifests |
| `log_workarounds` | `true` | Disable in high-traffic |
| `enable_metrics` | `true` | Always recommended |
| `log_sample_rate` | `1.0` | Reduce for high-traffic |

## Emergency Disable

If compatibility system causes issues:

```json
{
  "compatibility": {
    "enabled": false
  }
}
```

Or via environment variable:
```bash
export REGISTRY_COMPAT_ENABLED=false
```

Restart registry. All workarounds disabled, pure OCI behavior.

## Getting Help

1. **Check metrics**: `curl http://localhost:5005/metrics | grep compat`
2. **Check logs**: Look for `[COMPAT]` entries
3. **Read full docs**: `/Users/ryan/development/ads-registry/docs/COMPATIBILITY.md`
4. **File issue**: Include User-Agent, logs, and metrics

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-11 | Initial implementation with Docker 29.2.0 fix |

## References

- Full Documentation: [COMPATIBILITY.md](COMPATIBILITY.md)
- Testing Guide: [COMPATIBILITY-TESTING.md](COMPATIBILITY-TESTING.md)
- Integration Guide: [COMPATIBILITY-INTEGRATION.md](COMPATIBILITY-INTEGRATION.md)
- Architecture: [../internal/compat/ARCHITECTURE.md](../internal/compat/ARCHITECTURE.md)
