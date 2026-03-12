# Compatibility System Testing Guide

This guide covers testing strategies for the ADS Container Registry compatibility system.

## Testing Approach

The compatibility system uses multiple testing layers:

1. **Unit Tests**: Test individual components (detection, version matching, etc.)
2. **Integration Tests**: Test middleware flow with mock clients
3. **Client Simulation**: Test with actual client User-Agents
4. **Live Testing**: Test with real Docker/Podman/containerd clients
5. **Load Testing**: Verify performance impact under load

## Unit Testing

### Client Detection Tests

Test User-Agent parsing for various clients:

```go
package compat_test

import (
    "net/http/httptest"
    "testing"

    "github.com/ryan/ads-registry/internal/compat"
    "github.com/stretchr/testify/assert"
)

func TestDockerClientDetection(t *testing.T) {
    tests := []struct {
        name          string
        userAgent     string
        expectedName  string
        expectedVer   string
        expectedProto string
    }{
        {
            name:          "Docker 29.2.0",
            userAgent:     "Docker/29.2.0 (linux; go1.21.5)",
            expectedName:  "docker",
            expectedVer:   "29.2.0",
            expectedProto: "docker",
        },
        {
            name:          "Podman 3.4.1",
            userAgent:     "podman/3.4.1",
            expectedName:  "podman",
            expectedVer:   "3.4.1",
            expectedProto: "docker",
        },
        {
            name:          "Containerd 1.6.8",
            userAgent:     "containerd/v1.6.8",
            expectedName:  "containerd",
            expectedVer:   "1.6.8",
            expectedProto: "containerd",
        },
    }

    detector := compat.NewClientDetector()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest("GET", "/v2/", nil)
            req.Header.Set("User-Agent", tt.userAgent)

            client := detector.DetectClient(req)

            assert.Equal(t, tt.expectedName, client.Name)
            assert.Equal(t, tt.expectedVer, client.Version)
            assert.Equal(t, tt.expectedProto, client.Protocol)
        })
    }
}
```

### Version Matching Tests

Test version pattern matching:

```go
func TestVersionMatching(t *testing.T) {
    client := &compat.ClientInfo{
        Name:         "docker",
        Version:      "29.2.0",
        VersionMajor: 29,
        VersionMinor: 2,
        VersionPatch: 0,
    }

    tests := []struct {
        pattern  string
        expected bool
    }{
        {"29.2.0", true},      // Exact match
        {"29.*", true},        // Wildcard major
        {"29.2.*", true},      // Wildcard major.minor
        {"29", true},          // Major only
        {"29.2", true},        // Major.minor
        {"30.*", false},       // Different major
        {"29.3.*", false},     // Different minor
    }

    for _, tt := range tests {
        t.Run(tt.pattern, func(t *testing.T) {
            result := client.MatchesVersion(tt.pattern)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### Configuration Validation Tests

Test configuration parsing and validation:

```go
func TestConfigValidation(t *testing.T) {
    cfg := compat.Config{
        Enabled: true,
        TLSCompatibility: compat.TLSCompatibility{
            ForceHTTP1ForClients: []string{
                "Docker/29\\..*",
                "containerd/1\\.6\\..*",
            },
        },
    }

    err := cfg.Validate()
    assert.NoError(t, err)

    // Test compiled patterns
    assert.True(t, cfg.TLSCompatibility.ShouldForceHTTP1("Docker/29.2.0"))
    assert.True(t, cfg.TLSCompatibility.ShouldForceHTTP1("containerd/1.6.8"))
    assert.False(t, cfg.TLSCompatibility.ShouldForceHTTP1("Docker/30.0.0"))
}
```

## Integration Testing

### Middleware Integration Tests

Test the full middleware flow:

```go
func TestDocker29ManifestFixActivation(t *testing.T) {
    // Setup
    cfg := compat.DefaultConfig()
    cfg.DockerClientWorkarounds.EnableDocker29ManifestFix = true
    cfg.DockerClientWorkarounds.ExtraFlushes = 3

    middleware, err := compat.NewMiddleware(&cfg)
    assert.NoError(t, err)

    // Create a test handler that writes a manifest
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusCreated)
        w.Write([]byte(`{"test":"manifest"}`))
    })

    // Wrap with middleware
    wrapped := middleware.ClientDetectionMiddleware(
        middleware.CompatibilityMiddleware(handler),
    )

    // Create request simulating Docker 29.2.0
    req := httptest.NewRequest("PUT", "/v2/test/manifests/latest", nil)
    req.Header.Set("User-Agent", "Docker/29.2.0")

    // Record response
    rec := httptest.NewRecorder()

    // Execute
    wrapped.ServeHTTP(rec, req)

    // Verify
    assert.Equal(t, http.StatusCreated, rec.Code)

    // Verify client was detected
    clientInfo := compat.GetClientInfo(req.Context())
    assert.NotNil(t, clientInfo)
    assert.Equal(t, "docker", clientInfo.Name)
    assert.Equal(t, "29.2.0", clientInfo.Version)

    // Verify workaround was applied
    assert.Contains(t, clientInfo.Workarounds, "docker_29_manifest_fix")
}
```

### Header Workaround Tests

Test header modifications:

```go
func TestDistributionAPIVersionHeader(t *testing.T) {
    cfg := compat.DefaultConfig()
    cfg.HeaderWorkarounds.AlwaysSendDistributionAPIVersion = true

    middleware, _ := compat.NewMiddleware(&cfg)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Handler doesn't set the header
        w.WriteHeader(http.StatusOK)
    })

    wrapped := middleware.ClientDetectionMiddleware(
        middleware.CompatibilityMiddleware(handler),
    )

    req := httptest.NewRequest("GET", "/v2/", nil)
    req.Header.Set("User-Agent", "Docker/29.2.0")

    rec := httptest.NewRecorder()
    wrapped.ServeHTTP(rec, req)

    // Verify header was added
    assert.Equal(t, "registry/2.0", rec.Header().Get("Docker-Distribution-API-Version"))
}
```

## Client Simulation Testing

### Docker CLI Simulation

Test with curl simulating Docker client:

```bash
#!/bin/bash
# test-docker-29-simulation.sh

BASE_URL="http://localhost:5005"
TOKEN="your_jwt_token_here"

echo "=== Testing Docker 29.2.0 Client Simulation ==="

# 1. Base check
echo -e "\n1. Testing base check (/v2/)"
curl -v -H "User-Agent: Docker/29.2.0" \
  "${BASE_URL}/v2/"

# 2. Manifest upload
echo -e "\n2. Testing manifest upload"
MANIFEST='{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "size": 1234,
    "digest": "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
  },
  "layers": []
}'

curl -v -X PUT \
  -H "User-Agent: Docker/29.2.0" \
  -H "Content-Type: application/vnd.docker.distribution.manifest.v2+json" \
  -H "Authorization: Bearer ${TOKEN}" \
  --data-binary "${MANIFEST}" \
  "${BASE_URL}/v2/testorg/testapp/manifests/latest"

# 3. Check logs for workaround activation
echo -e "\n3. Check server logs for:"
echo "   [COMPAT] Client detected: docker/29.2.0"
echo "   [COMPAT] Activating workarounds for docker/29.2.0: [docker_29_manifest_fix]"

# 4. Check metrics
echo -e "\n4. Checking Prometheus metrics"
curl -s "${BASE_URL}/metrics" | grep compat
```

### Podman Simulation

```bash
#!/bin/bash
# test-podman-simulation.sh

BASE_URL="http://localhost:5005"
TOKEN="your_jwt_token_here"

echo "=== Testing Podman 3.4.1 Client Simulation ==="

# Test with malformed digest (sha256- instead of sha256:)
echo -e "\n1. Testing digest workaround"
curl -v -H "User-Agent: podman/3.4.1" \
  -H "Authorization: Bearer ${TOKEN}" \
  "${BASE_URL}/v2/testorg/testapp/blobs/sha256-abcdef1234567890"

# Should also work with correct format
curl -v -H "User-Agent: podman/3.4.1" \
  -H "Authorization: Bearer ${TOKEN}" \
  "${BASE_URL}/v2/testorg/testapp/blobs/sha256:abcdef1234567890"
```

### Containerd Simulation

```bash
#!/bin/bash
# test-containerd-simulation.sh

BASE_URL="http://localhost:5005"
TOKEN="your_jwt_token_here"

echo "=== Testing Containerd 1.6.8 Client Simulation ==="

# Test Content-Length workaround
curl -v -H "User-Agent: containerd/v1.6.8" \
  -H "Authorization: Bearer ${TOKEN}" \
  "${BASE_URL}/v2/testorg/testapp/manifests/latest"

# Check response has Content-Length even if empty
echo -e "\nVerify Content-Length header is present"
```

## Live Client Testing

### Docker 29.2.0 Real Test

```bash
#!/bin/bash
# test-real-docker-29.sh

# Prerequisites:
# - Docker 29.2.0 installed
# - Registry running on localhost:5005
# - Valid credentials configured

REGISTRY="localhost:5005"
IMAGE="${REGISTRY}/testorg/testapp:latest"

echo "=== Testing with Real Docker 29.2.0 Client ==="

# Verify Docker version
docker version --format '{{.Client.Version}}'

# Login
docker login ${REGISTRY}

# Build a test image
cat > Dockerfile <<EOF
FROM alpine:latest
RUN echo "test" > /test.txt
EOF

docker build -t ${IMAGE} .

# Push (this should trigger the workaround)
echo -e "\nPushing image (watch for workaround activation in logs)..."
docker push ${IMAGE}

# Verify push succeeded
if [ $? -eq 0 ]; then
    echo "✓ Push succeeded with Docker 29.2.0"
else
    echo "✗ Push failed - check server logs"
    exit 1
fi

# Pull to verify
docker rmi ${IMAGE}
docker pull ${IMAGE}

if [ $? -eq 0 ]; then
    echo "✓ Pull succeeded"
else
    echo "✗ Pull failed"
    exit 1
fi

# Cleanup
rm Dockerfile
echo -e "\n✓ All tests passed with real Docker 29.2.0 client"
```

### Podman Real Test

```bash
#!/bin/bash
# test-real-podman.sh

REGISTRY="localhost:5005"
IMAGE="${REGISTRY}/testorg/testapp:podman-test"

echo "=== Testing with Real Podman Client ==="

# Verify Podman version
podman version --format '{{.Client.Version}}'

# Login
podman login ${REGISTRY}

# Build and push
cat > Containerfile <<EOF
FROM alpine:latest
RUN echo "podman test" > /test.txt
EOF

podman build -t ${IMAGE} .
podman push ${IMAGE}

# Verify
if [ $? -eq 0 ]; then
    echo "✓ Podman push succeeded"
else
    echo "✗ Podman push failed"
    exit 1
fi
```

## Load Testing

### Compatibility System Performance Test

```bash
#!/bin/bash
# load-test-compatibility.sh

# Test performance impact of compatibility system

REGISTRY="localhost:5005"
CONCURRENCY=50
REQUESTS=1000

echo "=== Load Testing Compatibility System ==="

# 1. Test with workarounds enabled
echo -e "\n1. Testing WITH compatibility system (Docker 29.2.0 simulation)"
ab -n ${REQUESTS} -c ${CONCURRENCY} \
  -H "User-Agent: Docker/29.2.0" \
  -H "Authorization: Bearer ${TOKEN}" \
  "${REGISTRY}/v2/"

# 2. Test with different client (no workarounds)
echo -e "\n2. Testing WITHOUT workarounds (unknown client)"
ab -n ${REQUESTS} -c ${CONCURRENCY} \
  -H "User-Agent: UnknownClient/1.0.0" \
  -H "Authorization: Bearer ${TOKEN}" \
  "${REGISTRY}/v2/"

# 3. Check metrics for overhead
echo -e "\n3. Checking workaround duration metrics"
curl -s "${REGISTRY}/metrics" | grep workaround_duration_seconds

# 4. Compare response times
echo -e "\n4. Response time comparison:"
echo "   - With workarounds: check ab output above"
echo "   - Without workarounds: check ab output above"
echo "   - Expected overhead: < 1ms per request"
```

### Metrics Collection During Load Test

```bash
#!/bin/bash
# collect-metrics-during-load.sh

REGISTRY="localhost:5005"
DURATION=60  # seconds

echo "=== Collecting Metrics During Load Test ==="

# Start collecting metrics in background
while true; do
    curl -s "${REGISTRY}/metrics" | grep compat >> metrics-log.txt
    sleep 5
done &
METRICS_PID=$!

# Run load test
echo "Running load test for ${DURATION} seconds..."
ab -t ${DURATION} -c 50 \
  -H "User-Agent: Docker/29.2.0" \
  -H "Authorization: Bearer ${TOKEN}" \
  "${REGISTRY}/v2/"

# Stop metrics collection
kill ${METRICS_PID}

# Analyze metrics
echo -e "\n=== Metrics Analysis ==="
echo "Total workaround activations:"
grep workaround_activations_total metrics-log.txt | tail -1

echo "Average workaround duration:"
grep workaround_duration_seconds metrics-log.txt | \
  awk '{sum+=$2; count++} END {print sum/count " seconds"}'

echo "Client detection count:"
grep client_detections_total metrics-log.txt | tail -1
```

## Automated Test Suite

### Complete Test Script

```bash
#!/bin/bash
# run-all-compat-tests.sh

set -e

REGISTRY="localhost:5005"
TOKEN=""  # Set your token

echo "======================================"
echo "ADS Registry Compatibility Test Suite"
echo "======================================"

# 1. Unit tests (if using Go)
echo -e "\n[1/5] Running unit tests..."
cd /path/to/ads-registry
go test ./internal/compat/... -v

# 2. Client simulation tests
echo -e "\n[2/5] Running client simulation tests..."
./test-docker-29-simulation.sh
./test-podman-simulation.sh
./test-containerd-simulation.sh

# 3. Live client tests (if available)
echo -e "\n[3/5] Running live client tests..."
if command -v docker &> /dev/null; then
    ./test-real-docker-29.sh
else
    echo "Docker not available, skipping"
fi

if command -v podman &> /dev/null; then
    ./test-real-podman.sh
else
    echo "Podman not available, skipping"
fi

# 4. Load tests
echo -e "\n[4/5] Running load tests..."
./load-test-compatibility.sh

# 5. Metrics validation
echo -e "\n[5/5] Validating metrics..."
METRICS=$(curl -s "${REGISTRY}/metrics" | grep compat)

if [ -z "$METRICS" ]; then
    echo "✗ No compatibility metrics found!"
    exit 1
else
    echo "✓ Compatibility metrics present"
    echo "$METRICS" | head -10
fi

echo -e "\n======================================"
echo "✓ All compatibility tests completed!"
echo "======================================"
```

## Continuous Integration

### GitHub Actions Example

```yaml
# .github/workflows/compatibility-tests.yml
name: Compatibility Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  compatibility-tests:
    runs-on: ubuntu-latest

    steps:
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Run unit tests
      run: go test ./internal/compat/... -v -cover

    - name: Start registry
      run: |
        go build -o ads-registry ./cmd/ads-registry
        ./ads-registry serve &
        sleep 5

    - name: Run integration tests
      run: |
        chmod +x ./tests/compatibility/*.sh
        ./tests/compatibility/test-docker-29-simulation.sh
        ./tests/compatibility/test-podman-simulation.sh

    - name: Check metrics
      run: |
        curl http://localhost:5005/metrics | grep compat > metrics.txt
        cat metrics.txt

    - name: Upload metrics
      uses: actions/upload-artifact@v3
      with:
        name: compatibility-metrics
        path: metrics.txt
```

## Test Coverage Goals

Target coverage for the compatibility package:

- **Unit Tests**: 80%+ coverage
  - Client detection: 100%
  - Version matching: 100%
  - Configuration: 90%+

- **Integration Tests**: Key workflows
  - Docker 29.2.0 manifest fix
  - Header workarounds
  - Protocol overrides

- **Load Tests**: Performance validation
  - < 100μs overhead per request
  - No memory leaks over 1 hour
  - Metrics accuracy under load

## Troubleshooting Tests

### Tests Failing

**Issue**: Client detection tests failing

**Solution**:
1. Check User-Agent patterns in `detection.go`
2. Verify regex compilation in `config.go`
3. Add debug logging to `DetectClient()`

**Issue**: Workaround not activating in tests

**Solution**:
1. Verify configuration is loaded correctly
2. Check middleware order (detection before compatibility)
3. Verify version matching logic
4. Add debug logging to `determineWorkarounds()`

**Issue**: Load tests showing high overhead

**Solution**:
1. Profile with pprof
2. Check for regex compilation in hot path
3. Verify metrics recording isn't blocking
4. Reduce log sample rate

## Best Practices

1. **Test with real clients when possible**: Simulators are good, but real clients find edge cases
2. **Monitor metrics in tests**: Verify observability works
3. **Test degradation gracefully**: Ensure system works when compatibility is disabled
4. **Load test regularly**: Catch performance regressions early
5. **Document test scenarios**: Make tests reproducible

## References

- [Go Testing Package](https://pkg.go.dev/testing)
- [httptest Package](https://pkg.go.dev/net/http/httptest)
- [Apache Bench (ab)](https://httpd.apache.org/docs/2.4/programs/ab.html)
- [Prometheus Testing](https://prometheus.io/docs/prometheus/latest/getting_started/)
