# Helm Chart Support in ADS Container Registry

**Full OCI-compliant Helm chart storage with the Referrers API**

---

## Overview

The ADS Container Registry provides complete support for Helm charts as OCI artifacts. Store, version, and distribute Helm charts alongside your container images in a single registry.

**Features:**
- ✅ Full Helm 3+ OCI support
- ✅ Chart versioning and tagging
- ✅ OCI Referrers API for signatures and provenance
- ✅ Chart metadata tracking
- ✅ Multi-arch chart support
- ✅ Chart search and discovery
- ✅ Automatic vulnerability scanning
- ✅ Access control and quotas

---

## Quick Start

### 1. Package Your Chart

```bash
# Create or package a Helm chart
helm package mychart/

# Output: mychart-1.0.0.tgz
```

### 2. Push to Registry

```bash
# Login to registry
helm registry login apps.afterdarksys.com:5005 \
  --username admin \
  --password <your-password>

# Push chart
helm push mychart-1.0.0.tgz oci://apps.afterdarksys.com:5005/helm-charts

# Output: Pushed: apps.afterdarksys.com:5005/helm-charts/mychart:1.0.0
```

### 3. Pull and Install

```bash
# Pull chart
helm pull oci://apps.afterdarksys.com:5005/helm-charts/mychart --version 1.0.0

# Or install directly
helm install myrelease oci://apps.afterdarksys.com:5005/helm-charts/mychart --version 1.0.0
```

---

## Chart Organization

### Recommended Structure

```
apps.afterdarksys.com:5005/
├── helm-charts/               # Public/shared charts
│   ├── nginx:1.0.0
│   ├── postgres:2.1.0
│   └── redis:1.5.0
├── team-a/helm/               # Team-specific charts
│   ├── microservice-a:1.0.0
│   └── worker:2.0.0
└── platform/charts/           # Platform charts
    ├── monitoring:1.0.0
    └── logging:1.0.0
```

### Namespace Best Practices

| Namespace | Purpose | Example |
|-----------|---------|---------|
| `helm-charts` | Public/shared charts | `helm-charts/nginx:1.0.0` |
| `{team}/helm` | Team-specific charts | `backend/helm/api:1.0.0` |
| `platform/charts` | Platform/infrastructure | `platform/charts/istio:1.0.0` |
| `apps/{env}` | Environment-specific | `apps/prod/webapp:1.0.0` |

---

## Chart Operations

### Pushing Charts

```bash
# Basic push
helm push mychart-1.0.0.tgz oci://apps.afterdarksys.com:5005/helm-charts

# Push with specific tag
helm push mychart-1.0.0.tgz oci://apps.afterdarksys.com:5005/helm-charts/mychart:latest

# Push to specific namespace
helm push mychart-1.0.0.tgz oci://apps.afterdarksys.com:5005/myteam/charts
```

### Pulling Charts

```bash
# Pull specific version
helm pull oci://apps.afterdarksys.com:5005/helm-charts/mychart --version 1.0.0

# Pull latest
helm pull oci://apps.afterdarksys.com:5005/helm-charts/mychart

# Pull and untar
helm pull oci://apps.afterdarksys.com:5005/helm-charts/mychart --version 1.0.0 --untar
```

### Installing Charts

```bash
# Install from registry
helm install myrelease oci://apps.afterdarksys.com:5005/helm-charts/mychart --version 1.0.0

# Install with custom values
helm install myrelease oci://apps.afterdarksys.com:5005/helm-charts/mychart \
  --version 1.0.0 \
  --values custom-values.yaml

# Install to specific namespace
helm install myrelease oci://apps.afterdarksys.com:5005/helm-charts/mychart \
  --version 1.0.0 \
  --namespace production \
  --create-namespace
```

### Upgrading Charts

```bash
# Upgrade to new version
helm upgrade myrelease oci://apps.afterdarksys.com:5005/helm-charts/mychart --version 1.1.0

# Upgrade with values
helm upgrade myrelease oci://apps.afterdarksys.com:5005/helm-charts/mychart \
  --version 1.1.0 \
  --values new-values.yaml \
  --atomic \
  --timeout 5m
```

---

## Chart Discovery

### Using adsradm CLI

```bash
# List all Helm charts
adsradm artifacts list --type helm

# List charts in specific namespace
adsradm repos list | grep helm-charts

# View chart details
adsradm repos manifests helm-charts/mychart
```

### Using Helm (coming soon)

```bash
# Note: Traditional `helm repo add` is not supported for OCI registries
# Helm 3.8+ uses direct OCI references instead

# Instead, save registry URL as environment variable
export HELM_REGISTRY=oci://apps.afterdarksys.com:5005
export HELM_NAMESPACE=helm-charts

# Then install charts using the full OCI path
helm install myapp ${HELM_REGISTRY}/${HELM_NAMESPACE}/mychart:1.0.0
```

---

## Chart Signing and Verification

### Sign Charts with Cosign

```bash
# Sign a chart after pushing
cosign sign apps.afterdarksys.com:5005/helm-charts/mychart:1.0.0

# Verify signature
cosign verify apps.afterdarksys.com:5005/helm-charts/mychart:1.0.0 \
  --certificate-identity-regexp=.*@example.com \
  --certificate-oidc-issuer=https://github.com/login/oauth
```

### Attach Provenance

```bash
# Create provenance file
helm package mychart/ --sign --key mykey.asc --keyring ~/.gnupg/secring.gpg

# Attach provenance to chart (using ORAS)
oras attach apps.afterdarksys.com:5005/helm-charts/mychart:1.0.0 \
  --artifact-type application/vnd.cncf.helm.chart.provenance.v1+json \
  mychart-1.0.0.tgz.prov
```

### Discover Signatures and Provenance

```bash
# List all artifacts attached to a chart (uses OCI Referrers API)
curl -H "Authorization: Bearer $TOKEN" \
  https://apps.afterdarksys.com:5005/v2/helm-charts/mychart/referrers/sha256:abc123...

# Response:
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "digest": "sha256:def456...",
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "artifactType": "application/vnd.dev.cosign.simplesigning.v1+json",
      "size": 1234
    },
    {
      "digest": "sha256:ghi789...",
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "artifactType": "application/vnd.cncf.helm.chart.provenance.v1+json",
      "size": 5678
    }
  ]
}
```

---

## Chart Scanning

Charts are automatically scanned for vulnerabilities:

```bash
# View scan results
adsradm scans get sha256:abc123...

# Check for critical CVEs
adsradm scans list | grep CRITICAL
```

---

## CI/CD Integration

### GitHub Actions

```yaml
name: Publish Helm Chart

on:
  push:
    tags:
      - 'v*'

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install Helm
        uses: azure/setup-helm@v3
        with:
          version: '3.12.0'

      - name: Package chart
        run: |
          helm package charts/mychart

      - name: Login to registry
        run: |
          echo "${{ secrets.REGISTRY_PASSWORD }}" | \
          helm registry login apps.afterdarksys.com:5005 \
            --username ${{ secrets.REGISTRY_USERNAME }} \
            --password-stdin

      - name: Push chart
        run: |
          helm push mychart-*.tgz oci://apps.afterdarksys.com:5005/helm-charts

      - name: Sign chart with Cosign
        uses: sigstore/cosign-installer@v3
      - run: |
          cosign sign apps.afterdarksys.com:5005/helm-charts/mychart:${GITHUB_REF_NAME#v}
```

### GitLab CI

```yaml
publish-chart:
  image: alpine/helm:3.12.0
  script:
    - helm package charts/mychart
    - echo "$REGISTRY_PASSWORD" | helm registry login $REGISTRY_URL --username $REGISTRY_USERNAME --password-stdin
    - helm push mychart-*.tgz oci://$REGISTRY_URL/helm-charts
  only:
    - tags
```

---

## ArgoCD Integration

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: myapp
spec:
  source:
    chart: mychart
    repoURL: apps.afterdarksys.com:5005/helm-charts
    targetRevision: 1.0.0
    helm:
      values: |
        replicaCount: 3
        image:
          repository: apps.afterdarksys.com:5005/myapp/backend
          tag: latest
  destination:
    server: https://kubernetes.default.svc
    namespace: production
```

---

## Flux Integration

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: ads-registry
spec:
  type: oci
  url: oci://apps.afterdarksys.com:5005/helm-charts
  interval: 5m

---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: myapp
spec:
  interval: 5m
  chart:
    spec:
      chart: mychart
      version: '1.0.0'
      sourceRef:
        kind: HelmRepository
        name: ads-registry
  values:
    replicaCount: 3
```

---

## Access Control

### User Scopes

```bash
# Create Helm-specific user
adsradm users create helm-user --scopes="helm-charts/*:pull,push"

# Read-only access
adsradm users create helm-readonly --scopes="helm-charts/*:pull"

# Full access
adsradm users create helm-admin --scopes="helm-charts/*:*"
```

### Namespace Quotas

```bash
# Set quota for helm-charts namespace (10GB)
adsradm quotas set helm-charts 10737418240

# Check usage
adsradm quotas list
```

---

## Chart Versions and Tags

### Semantic Versioning

```bash
# Push version 1.0.0
helm push mychart-1.0.0.tgz oci://apps.afterdarksys.com:5005/helm-charts

# Push version 1.1.0
helm push mychart-1.1.0.tgz oci://apps.afterdarksys.com:5005/helm-charts

# List all versions
adsradm repos tags helm-charts/mychart
```

### Tagging Strategies

```bash
# Production release
helm push mychart-1.0.0.tgz oci://apps.afterdarksys.com:5005/helm-charts/mychart:1.0.0
helm push mychart-1.0.0.tgz oci://apps.afterdarksys.com:5005/helm-charts/mychart:latest
helm push mychart-1.0.0.tgz oci://apps.afterdarksys.com:5005/helm-charts/mychart:stable

# Pre-release
helm push mychart-1.1.0-rc1.tgz oci://apps.afterdarksys.com:5005/helm-charts/mychart:1.1.0-rc1
helm push mychart-1.1.0-rc1.tgz oci://apps.afterdarksys.com:5005/helm-charts/mychart:beta
```

---

## Migration from Traditional Helm Repos

### From ChartMuseum

```bash
# Export charts from ChartMuseum
for chart in $(curl http://chartmuseum.example.com/api/charts | jq -r 'keys[]'); do
  for version in $(curl http://chartmuseum.example.com/api/charts/$chart | jq -r '.[].version'); do
    curl -o ${chart}-${version}.tgz \
      http://chartmuseum.example.com/charts/${chart}-${version}.tgz
  done
done

# Push to ADS Registry
for tgz in *.tgz; do
  helm push $tgz oci://apps.afterdarksys.com:5005/helm-charts
done
```

### From Artifactory

```bash
# Use JFrog CLI to export
jfrog rt download helm-local/*.tgz ./charts/

# Push to ADS Registry
cd charts
for tgz in *.tgz; do
  helm push $tgz oci://apps.afterdarksys.com:5005/helm-charts
done
```

---

## Troubleshooting

### Common Issues

**Issue: "helm push" fails with authentication error**

```bash
# Solution: Login to registry
helm registry login apps.afterdarksys.com:5005 --username admin

# Or use environment variables
export HELM_REGISTRY_CONFIG=~/.config/helm/registry.json
```

**Issue: Chart not found after push**

```bash
# Verify chart was pushed
adsradm repos list | grep mychart

# Check tags
adsradm repos tags helm-charts/mychart

# Verify manifest
curl -H "Authorization: Bearer $TOKEN" \
  https://apps.afterdarksys.com:5005/v2/helm-charts/mychart/manifests/1.0.0
```

**Issue: Cannot install chart in ArgoCD**

```bash
# Ensure ArgoCD has registry credentials
kubectl create secret docker-registry helm-registry \
  --docker-server=apps.afterdarksys.com:5005 \
  --docker-username=admin \
  --docker-password=<password> \
  --namespace=argocd
```

---

## Best Practices

1. **Versioning**: Always use semantic versioning (1.0.0, 1.1.0, etc.)
2. **Namespaces**: Organize charts by team or project in separate namespaces
3. **Signing**: Sign production charts with Cosign
4. **Scanning**: Review vulnerability scan results before deployment
5. **Quotas**: Set namespace quotas to prevent runaway storage
6. **Tagging**: Use both version tags (1.0.0) and semantic tags (latest, stable)
7. **CI/CD**: Automate chart publishing in your CI/CD pipeline
8. **Provenance**: Attach provenance files for supply chain security

---

## API Reference

### Push Chart (via Helm CLI)

```bash
helm push <chart.tgz> oci://<registry>/<namespace>
```

### OCI Referrers API

```bash
GET /v2/{namespace}/{chart}/referrers/{digest}?artifactType={type}
```

### Artifact Listing

```bash
GET /api/v2/artifacts?type=application/vnd.cncf.helm.chart.content.v1.tar%2Bgzip
```

---

## Resources

- [Helm OCI Support](https://helm.sh/docs/topics/registries/)
- [OCI Distribution Spec](https://github.com/opencontainers/distribution-spec)
- [OCI Referrers API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#referrers-api)
- [Cosign Documentation](https://docs.sigstore.dev/cosign/overview/)
- [ORAS Documentation](https://oras.land/)

---

**Need help?** File an issue at [GitHub](https://github.com/ryan/ads-registry/issues)
