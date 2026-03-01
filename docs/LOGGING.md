# Enterprise Logging Setup

ADS Container Registry supports enterprise-grade logging with multiple backends for comprehensive observability.

## Overview

The registry can simultaneously send logs to:

1. **stdout** - Container logs (always enabled)
2. **Syslog** - Traditional log aggregation
3. **Elasticsearch** - Structured log analytics

All log destinations receive identical structured log entries with full context.

## Log Entry Structure

Each log entry includes:

```json
{
  "timestamp": "2026-03-02T10:15:30Z",
  "level": "INFO",
  "message": "HTTP Request",
  "service": "ads-registry",
  "hostname": "ads-registry-pod-abc123",
  "method": "PUT",
  "path": "/v2/myorg/nginx/manifests/latest",
  "status_code": 201,
  "duration_ms": 145.3,
  "user_agent": "docker/24.0.7",
  "remote_addr": "10.244.1.5:54321",
  "fields": {
    "namespace": "myorg",
    "repo": "nginx",
    "tag": "latest"
  }
}
```

## Syslog Configuration

### Local Syslog

Send logs to the local syslog daemon:

```json
{
  "logging": {
    "syslog": {
      "enabled": true,
      "server": "local",
      "tag": "ads-registry",
      "priority": "INFO"
    }
  }
}
```

### Remote Syslog (UDP)

Most common configuration for network logging:

```json
{
  "logging": {
    "syslog": {
      "enabled": true,
      "server": "udp://syslog.company.com:514",
      "tag": "ads-registry",
      "priority": "INFO"
    }
  }
}
```

### Remote Syslog (TCP)

For guaranteed delivery:

```json
{
  "logging": {
    "syslog": {
      "enabled": true,
      "server": "tcp://syslog.company.com:514",
      "tag": "ads-registry",
      "priority": "WARNING"
    }
  }
}
```

### Unix Domain Socket

For high-performance local logging:

```json
{
  "logging": {
    "syslog": {
      "enabled": true,
      "server": "unix:///var/run/syslog",
      "tag": "ads-registry",
      "priority": "DEBUG"
    }
  }
}
```

### Priority Levels

Available priority levels (from most to least verbose):

- `DEBUG` - Development debugging information
- `INFO` - Normal operational messages (default)
- `WARNING` - Warning conditions
- `ERROR` - Error conditions
- `CRITICAL` - Critical conditions requiring immediate attention

## Elasticsearch Configuration

### Basic Setup

```json
{
  "logging": {
    "elasticsearch": {
      "enabled": true,
      "endpoint": "http://elasticsearch:9200",
      "index": "ads-registry",
      "username": "",
      "password": ""
    }
  }
}
```

### With Authentication

```json
{
  "logging": {
    "elasticsearch": {
      "enabled": true,
      "endpoint": "https://elasticsearch.company.com:9200",
      "index": "container-registry",
      "username": "ads-registry",
      "password": "your-secure-password"
    }
  }
}
```

### Index Naming

Logs are automatically stored in date-suffixed indexes:

- Configuration: `"index": "ads-registry"`
- Actual indexes: `ads-registry-2026.03.01`, `ads-registry-2026.03.02`, etc.

This allows for easy index rotation and retention policies.

## Docker Deployment

### Docker Compose with Elasticsearch

```yaml
version: '3.8'
services:
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.11.0
    environment:
      - discovery.type=single-node
      - xpack.security.enabled=false
    ports:
      - "9200:9200"

  ads-registry:
    image: ads-registry:latest
    ports:
      - "5005:5005"
    volumes:
      - ./config.json:/app/config.json
      - ./data:/app/data
    depends_on:
      - elasticsearch
```

config.json:
```json
{
  "logging": {
    "elasticsearch": {
      "enabled": true,
      "endpoint": "http://elasticsearch:9200",
      "index": "ads-registry"
    }
  }
}
```

### Docker Compose with Rsyslog

```yaml
version: '3.8'
services:
  rsyslog:
    image: rsyslog/syslog_appliance_alpine
    ports:
      - "514:514/udp"
    volumes:
      - ./logs:/logs

  ads-registry:
    image: ads-registry:latest
    ports:
      - "5005:5005"
    volumes:
      - ./config.json:/app/config.json
      - ./data:/app/data
    depends_on:
      - rsyslog
```

config.json:
```json
{
  "logging": {
    "syslog": {
      "enabled": true,
      "server": "udp://rsyslog:514",
      "tag": "ads-registry",
      "priority": "INFO"
    }
  }
}
```

## Kubernetes Deployment

### With ELK Stack

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ads-registry
spec:
  template:
    spec:
      containers:
      - name: registry
        image: ads-registry:latest
        env:
        - name: ELASTICSEARCH_URL
          value: "http://elasticsearch.logging.svc.cluster.local:9200"
        - name: ELASTICSEARCH_USER
          valueFrom:
            secretKeyRef:
              name: elasticsearch-creds
              key: username
        - name: ELASTICSEARCH_PASS
          valueFrom:
            secretKeyRef:
              name: elasticsearch-creds
              key: password
        volumeMounts:
        - name: config
          mountPath: /app/config.json
          subPath: config.json
      volumes:
      - name: config
        configMap:
          name: registry-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: registry-config
data:
  config.json: |
    {
      "logging": {
        "elasticsearch": {
          "enabled": true,
          "endpoint": "http://elasticsearch.logging.svc.cluster.local:9200",
          "index": "ads-registry",
          "username": "${ELASTICSEARCH_USER}",
          "password": "${ELASTICSEARCH_PASS}"
        }
      }
    }
```

### With Fluentd Sidecar

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ads-registry
spec:
  template:
    spec:
      containers:
      - name: registry
        image: ads-registry:latest
        volumeMounts:
        - name: shared-logs
          mountPath: /var/log/ads-registry

      - name: fluentd
        image: fluent/fluentd:v1.16
        volumeMounts:
        - name: shared-logs
          mountPath: /var/log/ads-registry
        - name: fluentd-config
          mountPath: /fluentd/etc

      volumes:
      - name: shared-logs
        emptyDir: {}
      - name: fluentd-config
        configMap:
          name: fluentd-config
```

## Elasticsearch Index Template

Create an index template for better log management:

```bash
curl -X PUT "http://elasticsearch:9200/_index_template/ads-registry-logs" -H 'Content-Type: application/json' -d'
{
  "index_patterns": ["ads-registry-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 1,
      "index.lifecycle.name": "ads-registry-policy"
    },
    "mappings": {
      "properties": {
        "timestamp": { "type": "date" },
        "level": { "type": "keyword" },
        "message": { "type": "text" },
        "service": { "type": "keyword" },
        "hostname": { "type": "keyword" },
        "method": { "type": "keyword" },
        "path": { "type": "keyword" },
        "status_code": { "type": "integer" },
        "duration_ms": { "type": "float" },
        "user_agent": { "type": "text" },
        "remote_addr": { "type": "ip" }
      }
    }
  }
}
'
```

## Index Lifecycle Management

Set up ILM policy for automatic index rotation and deletion:

```bash
curl -X PUT "http://elasticsearch:9200/_ilm/policy/ads-registry-policy" -H 'Content-Type: application/json' -d'
{
  "policy": {
    "phases": {
      "hot": {
        "min_age": "0ms",
        "actions": {
          "rollover": {
            "max_primary_shard_size": "50gb",
            "max_age": "7d"
          }
        }
      },
      "warm": {
        "min_age": "7d",
        "actions": {
          "readonly": {}
        }
      },
      "delete": {
        "min_age": "90d",
        "actions": {
          "delete": {}
        }
      }
    }
  }
}
'
```

## Kibana Dashboards

### Create Index Pattern

1. Open Kibana
2. Go to Management → Stack Management → Index Patterns
3. Create index pattern: `ads-registry-*`
4. Select timestamp field: `timestamp`

### Useful Queries

**HTTP Errors:**
```
level: ERROR AND status_code: >= 400
```

**Slow Requests:**
```
duration_ms: > 1000
```

**Failed Authentication:**
```
message: "Unauthorized" OR message: "Forbidden"
```

**By Namespace:**
```
fields.namespace: "myorg"
```

## Log Levels by Component

| Component | Default Level | Purpose |
|-----------|--------------|---------|
| HTTP Requests | INFO | Track all API calls |
| Authentication | INFO | Login attempts, token generation |
| Policy Engine | WARNING | Only log denials |
| Scanner | INFO | Vulnerability scan results |
| Database | ERROR | Only log errors |
| Startup/Shutdown | INFO | Lifecycle events |

## Performance Considerations

### Elasticsearch
- Async logging prevents blocking HTTP requests
- Failed ES writes are logged to stdout
- 5-second timeout per request
- Connection pooling enabled

### Syslog
- Synchronous (may block on slow networks)
- Consider using TCP for reliability
- Use local Unix socket for best performance

### High-Traffic Scenarios

For >10,000 req/min:

1. **Disable INFO-level HTTP logging**
   ```json
   {
     "logging": {
       "syslog": {
         "priority": "WARNING"
       }
     }
   }
   ```

2. **Use local buffering**
   - Local syslog with rsyslog forwarder
   - Reduces network overhead

3. **Elasticsearch optimizations**
   - Increase bulk indexing
   - Use SSD storage
   - Proper shard sizing

## Troubleshooting

### Syslog not receiving logs

Check connectivity:
```bash
nc -zv syslog.company.com 514
```

Test UDP:
```bash
echo "test" | nc -u syslog.company.com 514
```

### Elasticsearch connection failed

Verify endpoint:
```bash
curl http://elasticsearch:9200/_cluster/health
```

Check authentication:
```bash
curl -u username:password http://elasticsearch:9200
```

### Logs not appearing

Check stdout:
```bash
docker logs ads-registry
```

Enable debug logging:
```json
{
  "logging": {
    "syslog": {
      "priority": "DEBUG"
    }
  }
}
```

## Example Log Queries

### Grep stdout logs

```bash
# All errors
docker logs ads-registry 2>&1 | grep "\[ERROR\]"

# Specific namespace
docker logs ads-registry 2>&1 | grep "namespace=myorg"

# Slow requests
docker logs ads-registry 2>&1 | grep "duration_ms" | awk -F'=' '$2 > 1000'
```

### Elasticsearch queries

```bash
# Recent errors
curl -X GET "http://elasticsearch:9200/ads-registry-*/_search?pretty" -H 'Content-Type: application/json' -d'
{
  "query": {
    "bool": {
      "must": [
        { "term": { "level": "ERROR" } },
        { "range": { "timestamp": { "gte": "now-1h" } } }
      ]
    }
  }
}
'

# Top 10 slowest requests
curl -X GET "http://elasticsearch:9200/ads-registry-*/_search?pretty" -H 'Content-Type: application/json' -d'
{
  "size": 10,
  "sort": [{ "duration_ms": "desc" }],
  "query": { "exists": { "field": "duration_ms" } }
}
'
```

## Security Best Practices

1. **Use TLS** for remote syslog and Elasticsearch
2. **Authenticate** - Always use username/password for Elasticsearch
3. **Rotate credentials** - Change ES passwords regularly
4. **Network segmentation** - Restrict access to logging infrastructure
5. **Monitor access** - Audit who accesses logs
6. **PII scrubbing** - Avoid logging sensitive data
7. **Retention policies** - Delete old logs per compliance requirements

---

**By Ryan and the team at After Dark Systems, LLC.**
