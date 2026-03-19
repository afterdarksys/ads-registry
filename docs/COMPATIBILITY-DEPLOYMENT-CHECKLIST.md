# Compatibility System - Deployment Checklist

## Pre-Deployment

### 1. Code Review
- [ ] Review all files in `/Users/ryan/development/ads-registry/internal/compat/`
- [ ] Verify middleware integration in `cmd/ads-registry/cmd/serve.go`
- [ ] Check configuration schema in `internal/config/config.go`
- [ ] Review documentation for accuracy

### 2. Build Verification
```bash
cd /Users/ryan/development/ads-registry
go mod tidy
go build -o ads-registry ./cmd/ads-registry
```
- [ ] Build completes without errors
- [ ] No import issues
- [ ] Binary created successfully

### 3. Configuration Preparation
- [ ] Copy `config-examples/compatibility-minimal.json` as starting point
- [ ] Review `docs/COMPATIBILITY.md` for configuration options
- [ ] Customize for your environment
- [ ] Validate JSON syntax

### 4. Testing (Staging)
- [ ] Deploy to staging environment
- [ ] Verify startup logs show:
  ```
  [INFO] Compatibility system enabled (Postfix-style client workarounds)
  ```
- [ ] Test with curl simulation:
  ```bash
  curl -H "User-Agent: Docker/29.2.0" http://staging:5005/v2/
  ```
- [ ] Check metrics endpoint:
  ```bash
  curl http://staging:5005/metrics | grep compat
  ```
- [ ] Test actual Docker 29.2.0 push if available

## Deployment Steps

### 1. Backup Current Configuration
```bash
cp /path/to/config.json /path/to/config.json.backup.$(date +%Y%m%d)
```
- [ ] Configuration backed up
- [ ] Backup location documented

### 2. Update Configuration

Add to `config.json`:
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
      "log_client_detection": true,
      "enable_metrics": true,
      "log_sample_rate": 1.0
    }
  }
}
```
- [ ] Configuration updated
- [ ] JSON validated
- [ ] Changes reviewed

### 3. Deploy New Binary
```bash
# Build
go build -o ads-registry ./cmd/ads-registry

# Deploy (adjust paths as needed)
sudo systemctl stop ads-registry
sudo cp ads-registry /usr/local/bin/
sudo systemctl start ads-registry
```
- [ ] Service stopped gracefully
- [ ] Binary deployed
- [ ] Service started

### 4. Verify Deployment

Check logs:
```bash
sudo journalctl -u ads-registry -f | grep COMPAT
```

Expected output:
```
[INFO] Compatibility system enabled (Postfix-style client workarounds)
[COMPAT] Client detected: docker/29.2.0 ...
[COMPAT] Activating workarounds for docker/29.2.0: [docker_29_manifest_fix]
```

- [ ] Service started successfully
- [ ] Compatibility system enabled
- [ ] No error messages

### 5. Verify Metrics

```bash
curl http://localhost:5005/metrics | grep compat
```

Expected metrics:
```
ads_registry_compat_client_detections_total{...}
ads_registry_compat_workaround_activations_total{...}
```

- [ ] Metrics endpoint accessible
- [ ] Compatibility metrics present
- [ ] Prometheus scraping configured

## Post-Deployment

### 1. Functional Testing

Test Docker push:
```bash
docker login localhost:5005
docker pull alpine:latest
docker tag alpine:latest localhost:5005/test/alpine:latest
docker push localhost:5005/test/alpine:latest
```

- [ ] Login succeeds
- [ ] Push succeeds
- [ ] Check logs for client detection
- [ ] Verify no errors

### 2. Monitor for 1 Hour

Watch logs:
```bash
tail -f /var/log/ads-registry.log | grep -E "(ERROR|COMPAT)"
```

Monitor metrics:
```bash
watch -n 10 'curl -s http://localhost:5005/metrics | grep compat_workaround_activations_total'
```

- [ ] No unexpected errors
- [ ] Workarounds activating as expected
- [ ] Client detection working
- [ ] Performance acceptable

### 3. Client Distribution Analysis

After 1 hour, analyze client distribution:
```bash
curl -s http://localhost:5005/metrics | grep client_detections_total
```

- [ ] Document detected clients
- [ ] Note versions in use
- [ ] Identify any unknown clients
- [ ] Plan workarounds if needed

### 4. Performance Validation

Check workaround duration:
```bash
curl -s http://localhost:5005/metrics | grep workaround_duration_seconds
```

Expected: p99 < 1ms

- [ ] Performance impact acceptable
- [ ] No latency spikes
- [ ] Memory usage normal

### 5. Adjust Configuration

Based on observations, tune settings:

High-traffic? Reduce logging:
```json
{
  "observability": {
    "log_workarounds": false,
    "log_client_detection": false,
    "log_sample_rate": 0.01
  }
}
```

Specific client issues? Enable specific workarounds:
```json
{
  "broken_client_hacks": {
    "podman_digest_workaround": true
  }
}
```

- [ ] Configuration tuned
- [ ] Changes documented
- [ ] Service restarted if needed

## Monitoring Setup

### 1. Prometheus Configuration

Add to `prometheus.yml`:
```yaml
scrape_configs:
  - job_name: 'ads-registry'
    static_configs:
      - targets: ['localhost:5005']
    scrape_interval: 15s
```

- [ ] Prometheus configured
- [ ] Scraping working
- [ ] Targets healthy

### 2. Grafana Dashboard

Create dashboard with panels:

**Client Distribution**:
```promql
sum by (client, version) (
  increase(ads_registry_compat_client_detections_total[1h])
)
```

**Workaround Activation Rate**:
```promql
rate(ads_registry_compat_workaround_activations_total[5m])
```

**Performance Impact**:
```promql
histogram_quantile(0.99,
  rate(ads_registry_compat_workaround_duration_seconds_bucket[5m])
)
```

- [ ] Grafana dashboard created
- [ ] Panels configured
- [ ] Alerts set up

### 3. Alerting Rules

Add Prometheus alerts:
```yaml
groups:
  - name: compatibility
    rules:
      - alert: HighWorkaroundActivationRate
        expr: rate(ads_registry_compat_workaround_activations_total[5m]) > 100
        for: 5m
        annotations:
          summary: "High workaround activation rate"

      - alert: UnknownClientDetected
        expr: increase(ads_registry_compat_client_detections_total{client="unknown"}[1h]) > 10
        annotations:
          summary: "Unknown clients detected"
```

- [ ] Alert rules configured
- [ ] Alert routing set up
- [ ] Test alerts

## Documentation

### 1. Team Documentation
- [ ] Share `docs/COMPATIBILITY.md` with team
- [ ] Share `docs/COMPATIBILITY-QUICK-REFERENCE.md` for quick fixes
- [ ] Document deployment in runbook
- [ ] Add to incident response procedures

### 2. Operational Runbook

Create runbook entry:

**Title**: Compatibility System Troubleshooting

**Common Issues**:
- Docker 29.2.0 errors → Check `enable_docker_29_manifest_fix`
- Unknown clients → Check User-Agent in logs
- High overhead → Reduce `log_sample_rate`

**Emergency Disable**:
```json
{"compatibility": {"enabled": false}}
```
Restart registry.

- [ ] Runbook created
- [ ] Accessible to on-call
- [ ] Tested procedures

### 3. Changelog

Document in CHANGELOG.md:
```
## [Version X.Y.Z] - 2026-03-11

### Added
- Compatibility system for handling broken container registry clients
- Automatic client detection (Docker, Podman, containerd, etc.)
- Docker 29.2.0 manifest upload fix
- Comprehensive Prometheus metrics for compatibility events
- Configurable workarounds for known client bugs

### Changed
- Middleware chain now includes client detection and compatibility layers
- Configuration schema extended with `compatibility` section

### Fixed
- Docker 29.2.0 "short copy" manifest upload errors
```

- [ ] Changelog updated
- [ ] Version tagged
- [ ] Release notes prepared

## Rollback Plan

If issues occur:

### 1. Quick Disable

Edit `config.json`:
```json
{
  "compatibility": {
    "enabled": false
  }
}
```

Restart:
```bash
sudo systemctl restart ads-registry
```

- [ ] Rollback procedure documented
- [ ] Team knows how to disable
- [ ] Restart command accessible

### 2. Full Rollback

Restore previous binary:
```bash
sudo systemctl stop ads-registry
sudo cp /usr/local/bin/ads-registry.backup /usr/local/bin/ads-registry
sudo cp config.json.backup config.json
sudo systemctl start ads-registry
```

- [ ] Backup binary available
- [ ] Backup config available
- [ ] Rollback tested in staging

## Success Criteria

After 24 hours:

- [ ] No critical errors related to compatibility system
- [ ] Docker 29.2.0 pushes succeeding (if applicable)
- [ ] Client detection accuracy > 90%
- [ ] Performance impact < 0.1%
- [ ] Metrics being collected
- [ ] Team comfortable with system
- [ ] Documentation accessible

## Long-Term Maintenance

### Weekly
- [ ] Review metrics for unknown clients
- [ ] Check for new client versions
- [ ] Verify workarounds still needed

### Monthly
- [ ] Review workaround activation rates
- [ ] Update documentation if needed
- [ ] Check for client bug fixes
- [ ] Consider disabling obsolete workarounds

### Quarterly
- [ ] Full compatibility audit
- [ ] Performance analysis
- [ ] Configuration optimization
- [ ] Team training refresh

## Support Contacts

| Role | Contact | Responsibility |
|------|---------|---------------|
| Primary Engineer | [Name] | Compatibility system owner |
| On-Call | [Contact] | 24/7 support |
| Team Lead | [Name] | Escalation point |

## References

- **Full Documentation**: `/Users/ryan/development/ads-registry/docs/COMPATIBILITY.md`
- **Quick Reference**: `/Users/ryan/development/ads-registry/docs/COMPATIBILITY-QUICK-REFERENCE.md`
- **Testing Guide**: `/Users/ryan/development/ads-registry/docs/COMPATIBILITY-TESTING.md`
- **Integration Guide**: `/Users/ryan/development/ads-registry/docs/COMPATIBILITY-INTEGRATION.md`
- **Implementation Summary**: `/Users/ryan/development/ads-registry/COMPATIBILITY-SYSTEM-SUMMARY.md`

## Sign-off

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Developer | | | |
| Reviewer | | | |
| DevOps | | | |
| Manager | | | |

---

**Deployment Date**: _______________
**Deployed By**: _______________
**Deployment Status**: [ ] Success [ ] Partial [ ] Rollback
**Notes**: _______________________________________________________________
