# TCP Tuning Guide for ADS Registry

This guide covers optimizations for handling large container images and preventing connection drops during transfers.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Application-Level Settings](#application-level-settings)
- [OS-Level TCP Tuning](#os-level-tcp-tuning)
- [Reverse Proxy Configuration](#reverse-proxy-configuration)
- [Monitoring and Troubleshooting](#monitoring-and-troubleshooting)
- [Benchmarking](#benchmarking)

## Overview

Container images can be **very large** (multi-GB), and without proper TCP tuning, you may experience:
- Connection timeouts during large uploads/downloads
- Dropped connections for slow clients
- Poor throughput on high-latency networks
- Port exhaustion under heavy load

This guide provides comprehensive tuning at multiple levels.

## Quick Start

### Automated Setup (Linux)

```bash
# Run the automated TCP tuning script (requires sudo)
sudo ./scripts/tune-tcp.sh

# Rebuild and restart the registry
./build.sh --skip-test
./ads-registry serve
```

### Manual Configuration

If you prefer to configure manually, follow the sections below.

## Application-Level Settings

### Config File Settings

The registry's timeout and buffer settings are configured in `config.json` or `config.production.json`:

```json
{
  "server": {
    "address": "0.0.0.0",
    "port": 5005,
    "read_timeout": 600000000000,        // 10 minutes (in nanoseconds)
    "write_timeout": 600000000000,       // 10 minutes (in nanoseconds)
    "idle_timeout": 120000000000,        // 2 minutes (in nanoseconds)
    "read_header_timeout": 30000000000,  // 30 seconds (in nanoseconds)
    "max_header_bytes": 52428800,        // 50MB (for large manifests)
    "tls": {
      "enabled": true,
      "port": 5006,
      "cert_file": "certs/server.crt",
      "key_file": "certs/server.key"
    }
  }
}
```

### Timeout Recommendations by Image Size

| Image Size | read_timeout | write_timeout | Use Case |
|------------|--------------|---------------|----------|
| < 500MB    | 300s (5m)    | 300s (5m)     | Standard images |
| 500MB-2GB  | 600s (10m)   | 600s (10m)    | Large images (default) |
| 2GB-10GB   | 1800s (30m)  | 1800s (30m)   | Very large images |
| > 10GB     | 3600s (60m)  | 3600s (60m)   | Massive images |

**Note:** Timeouts are in nanoseconds in the config file. Multiply seconds by 1,000,000,000.

Examples:
```
5 minutes  = 300 seconds  = 300000000000 nanoseconds
10 minutes = 600 seconds  = 600000000000 nanoseconds
30 minutes = 1800 seconds = 1800000000000 nanoseconds
```

### MaxHeaderBytes Setting

The `max_header_bytes` setting limits the size of HTTP headers. For registries with images that have many layers (100+ layers), you may need to increase this:

- **Default**: 10MB (10485760) - sufficient for most images
- **Large manifests**: 50MB (52428800) - images with 100-500 layers
- **Extreme**: 100MB (104857600) - images with 500+ layers

## OS-Level TCP Tuning

### Linux TCP Settings (sysctl)

The `tune-tcp.sh` script applies these settings automatically. For manual configuration, add to `/etc/sysctl.d/99-ads-registry.conf`:

#### Critical Settings for Large Transfers

```bash
# Increase TCP buffer sizes to 128MB
# Essential for high-bandwidth transfers
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 131072 134217728
net.ipv4.tcp_wmem = 4096 131072 134217728

# Enable TCP window scaling (required for buffers > 64KB)
net.ipv4.tcp_window_scaling = 1

# Disable slow start after idle
# Prevents bandwidth drop during long transfers
net.ipv4.tcp_slow_start_after_idle = 0
```

#### Connection Management

```bash
# Increase connection queue sizes
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 8192
net.core.netdev_max_backlog = 16384

# Optimize keepalive for detecting dead connections
net.ipv4.tcp_keepalive_time = 600
net.ipv4.tcp_keepalive_intvl = 10
net.ipv4.tcp_keepalive_probes = 6

# Allow TIME_WAIT socket reuse
net.ipv4.tcp_tw_reuse = 1

# Expand port range
net.ipv4.ip_local_port_range = 10000 65535
```

#### Apply Settings

```bash
# Apply immediately
sudo sysctl -p /etc/sysctl.d/99-ads-registry.conf

# Verify
sysctl net.core.rmem_max net.core.wmem_max
sysctl net.ipv4.tcp_rmem
```

### BBR Congestion Control (Recommended)

BBR (Bottleneck Bandwidth and RTT) provides better throughput than the default CUBIC algorithm, especially for high-latency networks.

**Requirements:** Linux kernel 4.9+

```bash
# Check if BBR is available
cat /proc/sys/net/ipv4/tcp_available_congestion_control

# Enable BBR
sudo modprobe tcp_bbr
echo "net.core.default_qdisc = fq" | sudo tee -a /etc/sysctl.d/99-ads-registry.conf
echo "net.ipv4.tcp_congestion_control = bbr" | sudo tee -a /etc/sysctl.d/99-ads-registry.conf
sudo sysctl -p /etc/sysctl.d/99-ads-registry.conf

# Verify
sysctl net.ipv4.tcp_congestion_control
```

### File Descriptor Limits

Add to `/etc/security/limits.d/99-ads-registry.conf`:

```bash
*               soft    nofile          65536
*               hard    nofile          65536
registry        soft    nofile          65536
registry        hard    nofile          65536
```

Verify after login:
```bash
ulimit -n
```

## Reverse Proxy Configuration

### NGINX (Recommended)

If using NGINX as a reverse proxy, configure these settings in your NGINX config:

```nginx
upstream ads_registry {
    server 127.0.0.1:5005;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name registry.example.com;

    # SSL configuration
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # Critical timeouts for large images
    proxy_connect_timeout 60s;
    proxy_send_timeout 600s;      # 10 minutes
    proxy_read_timeout 600s;      # 10 minutes
    send_timeout 600s;

    # Buffer settings
    client_max_body_size 10G;     # Max image size
    client_body_buffer_size 256k;
    proxy_buffering off;          # Disable buffering for streaming

    # Headers
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # Keepalive
    proxy_http_version 1.1;
    proxy_set_header Connection "";

    # Disable request/response buffering
    proxy_request_buffering off;
    proxy_buffering off;

    location / {
        proxy_pass http://ads_registry;
    }

    # Specific settings for blob uploads
    location ~ ^/v2/.*/blobs/uploads/ {
        proxy_pass http://ads_registry;
        client_max_body_size 0;       # No limit
        proxy_read_timeout 1800s;     # 30 minutes
        proxy_send_timeout 1800s;
    }

    # Specific settings for manifest uploads
    location ~ ^/v2/.*/manifests/ {
        proxy_pass http://ads_registry;
        proxy_read_timeout 300s;      # 5 minutes
        proxy_send_timeout 300s;
    }
}
```

### Caddy

```caddy
registry.example.com {
    reverse_proxy localhost:5005 {
        # Timeouts
        transport http {
            read_timeout 10m
            write_timeout 10m
            dial_timeout 60s
        }

        # Headers
        header_up X-Forwarded-Proto {scheme}
        header_up X-Real-IP {remote_host}
    }

    # File upload size limit
    request_body {
        max_size 10GB
    }
}
```

### Traefik

```yaml
http:
  routers:
    registry:
      rule: "Host(`registry.example.com`)"
      service: registry
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt

  services:
    registry:
      loadBalancer:
        servers:
          - url: "http://localhost:5005"

        # Timeouts
        responseForwarding:
          flushInterval: -1  # Disable buffering

        # Health check
        healthCheck:
          path: /health/live
          interval: 30s
          timeout: 5s

# Global timeouts in traefik.yml
respondingTimeouts:
  readTimeout: 600s
  writeTimeout: 600s
  idleTimeout: 120s
```

## Monitoring and Troubleshooting

### Check Current TCP Settings

```bash
# Buffer sizes
sysctl net.core.rmem_max net.core.wmem_max
sysctl net.ipv4.tcp_rmem net.ipv4.tcp_wmem

# Connection settings
sysctl net.core.somaxconn net.ipv4.tcp_max_syn_backlog

# Congestion control
sysctl net.ipv4.tcp_congestion_control

# File descriptors
ulimit -n
cat /proc/sys/fs/file-max
```

### Monitor Active Connections

```bash
# Show established connections
ss -tan | grep ESTAB | wc -l

# Show TIME_WAIT connections
ss -tan | grep TIME-WAIT | wc -l

# Monitor connection states
watch -n1 'ss -tan | tail -n +2 | awk "{print \$1}" | sort | uniq -c'

# Check registry connections
ss -tanp | grep ads-registry
```

### Test Upload Performance

```bash
# Test upload with timing
time docker push registry.example.com/test/large-image:latest

# Test with verbose output
docker push registry.example.com/test/large-image:latest --debug

# Monitor network throughput
iftop -i eth0
# or
nload eth0
```

### Registry Logs

```bash
# Watch for timeout errors
journalctl -u ads-registry -f | grep -i timeout

# Check for connection resets
journalctl -u ads-registry -f | grep -i "connection reset"

# Monitor slow requests
journalctl -u ads-registry -f | grep -E "took [0-9]+s"
```

### Common Issues

#### 1. Connection Timeout During Upload

**Symptoms:** `i/o timeout`, `connection reset by peer`

**Solutions:**
- Increase `write_timeout` in config
- Increase NGINX `proxy_send_timeout`
- Check OS TCP buffer sizes

#### 2. Connection Timeout During Download

**Symptoms:** `read timeout`, client reports timeout

**Solutions:**
- Increase `read_timeout` in config
- Increase NGINX `proxy_read_timeout`
- Verify network bandwidth

#### 3. "Request Entity Too Large" (413)

**Symptoms:** Upload fails with 413 error

**Solutions:**
- Increase NGINX `client_max_body_size`
- Increase registry `max_header_bytes` if many layers

#### 4. Port Exhaustion

**Symptoms:** `cannot assign requested address`

**Solutions:**
- Increase `net.ipv4.ip_local_port_range`
- Enable `net.ipv4.tcp_tw_reuse`
- Check for connection leaks

## Benchmarking

### Test TCP Buffer Performance

```bash
# Install iperf3
sudo apt-get install iperf3

# On server
iperf3 -s

# On client
iperf3 -c registry.example.com -t 60
```

Expected results:
- **Without tuning:** 100-500 Mbps
- **With tuning:** 1-10 Gbps (depending on network)

### Test Registry Upload Speed

```bash
# Create a large test image
dd if=/dev/zero of=testfile bs=1M count=1000  # 1GB file
docker build -t test-image:large - <<EOF
FROM busybox
COPY testfile /
EOF

# Time the push
time docker push registry.example.com/test/test-image:large

# Calculate throughput
# Throughput = Image Size / Time
```

### Load Testing

```bash
# Use docker-stress to simulate multiple concurrent pushes
git clone https://github.com/spotify/docker-stress
cd docker-stress

# Run 10 concurrent pushes
./docker-stress push \
  --image registry.example.com/test/nginx:latest \
  --concurrent 10 \
  --iterations 5
```

## Summary of Optimizations

| Layer | Setting | Default | Optimized | Impact |
|-------|---------|---------|-----------|--------|
| App | read_timeout | 10s | 600s | ⭐⭐⭐ High |
| App | write_timeout | 10s | 600s | ⭐⭐⭐ High |
| App | max_header_bytes | 10MB | 50MB | ⭐⭐ Medium |
| OS | tcp_rmem (max) | 4MB | 128MB | ⭐⭐⭐ High |
| OS | tcp_wmem (max) | 4MB | 128MB | ⭐⭐⭐ High |
| OS | somaxconn | 128 | 4096 | ⭐⭐ Medium |
| OS | tcp_slow_start_after_idle | 1 | 0 | ⭐⭐ Medium |
| OS | tcp_congestion_control | cubic | bbr | ⭐⭐ Medium |
| Proxy | proxy_read_timeout | 60s | 600s | ⭐⭐⭐ High |
| Proxy | client_max_body_size | 1m | 10g | ⭐⭐⭐ High |

## Additional Resources

- [Linux TCP Tuning Guide](https://fasterdata.es.net/host-tuning/linux/)
- [BBR Congestion Control](https://queue.acm.org/detail.cfm?id=3022184)
- [Docker Registry Specification](https://docs.docker.com/registry/spec/api/)
- [NGINX Reverse Proxy Tuning](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)

## Quick Reference Commands

```bash
# Apply all tuning
sudo ./scripts/tune-tcp.sh

# Check if BBR is enabled
sysctl net.ipv4.tcp_congestion_control

# Monitor connections
ss -tan | grep ESTAB | wc -l

# Test network throughput
iperf3 -c registry.example.com

# View registry logs
journalctl -u ads-registry -f

# Rebuild after config changes
./build.sh --skip-test
./ads-registry serve
```

---

**Last Updated:** 2026-03-21
**Author:** After Dark Systems, LLC
