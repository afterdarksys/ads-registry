## Supply Chain Security Analysis

**Comprehensive gap analysis for software supply chain security compliance**

---

## Table of Contents

1. [Overview](#overview)
2. [What is Supply Chain Security?](#what-is-supply-chain-security)
3. [Analysis Components](#analysis-components)
4. [Maturity Levels](#maturity-levels)
5. [Compliance Frameworks](#compliance-frameworks)
6. [Using the Analyzer](#using-the-analyzer)
7. [Remediation Guide](#remediation-guide)
8. [Best Practices](#best-practices)

---

## Overview

The ADS Container Registry includes a **Software Supply Chain Security Analyzer** that evaluates your container images against industry best practices and compliance frameworks including:

- **SLSA** (Supply-chain Levels for Software Artifacts)
- **NIST SSDF** (Secure Software Development Framework)
- **CIS Docker Benchmark**
- **Custom organizational policies**

**Key Features:**

✅ **SBOM (Software Bill of Materials)** validation
✅ **Provenance attestation** verification (in-toto, SLSA)
✅ **Signature verification** (Cosign, Sigstore)
✅ **Dependency risk analysis** (malware, typosquatting, licensing)
✅ **Gap identification** with remediation steps
✅ **Maturity scoring** (0-100 scale)
✅ **Compliance reporting** (SLSA, SSDF, CIS)

---

## What is Supply Chain Security?

Supply chain attacks have become one of the most serious threats to software security:

### Recent High-Profile Attacks

- **SolarWinds (2020)**: Compromised build system → 18,000 customers affected
- **Codecov (2021)**: Modified Bash uploader → credentials stolen
- **Log4Shell (2021)**: Vulnerability in widely-used dependency
- **UA-Parser-JS (2021)**: NPM package hijacked with crypto miner

### The Problem

Traditional security focuses on:
- ✅ Vulnerability scanning
- ✅ Runtime protection
- ❌ **Build integrity**
- ❌ **Dependency provenance**
- ❌ **Artifact authentication**

### The Solution

Supply chain security addresses:
- **Where** did this code come from? *(Provenance)*
- **What** is inside this artifact? *(SBOM)*
- **Who** built it? *(Signatures)*
- **Can I trust** the dependencies? *(Dependency analysis)*

---

## Analysis Components

The analyzer evaluates four key areas:

### 1. SBOM (Software Bill of Materials)

**What it is:**
A complete inventory of all components in your container image.

**Why it matters:**
- Know exactly what's in your images
- Track licenses and legal obligations
- Rapid response to newly discovered vulnerabilities
- Supply chain transparency

**What we check:**
- ✅ SBOM present
- ✅ Format (SPDX, CycloneDX, Syft)
- ✅ Completeness (% of dependencies documented)
- ✅ License information included
- ✅ Vulnerability data embedded
- ✅ Relationship graph (dependency tree)

**Scoring:**
- SBOM present: +15 points
- Completeness × 15/100: up to +15 points
- **Total: 30 points**

**Example findings:**
```json
{
  "sbom": {
    "present": true,
    "format": "cyclonedx",
    "version": "1.4",
    "component_count": 247,
    "package_managers": ["npm", "pip", "go"],
    "has_licenses": true,
    "has_vulnerability_data": true,
    "completeness": 94
  }
}
```

---

### 2. Provenance (Build Attestation)

**What it is:**
Cryptographically signed metadata about how your image was built.

**Why it matters:**
- Prove the image came from your build system
- Detect tampering or unauthorized builds
- Meet compliance requirements (SOC 2, FedRAMP)
- Enable policy enforcement

**What we check:**
- ✅ Provenance attestation present
- ✅ Format (in-toto, SLSA)
- ✅ SLSA level (0-4)
- ✅ Builder information
- ✅ Source repository and commit
- ✅ Cryptographic verification

**Scoring:**
- Provenance present: +10 points
- Verified: +10 points
- SLSA level × 2.5: up to +10 points
- **Total: 30 points**

**SLSA Levels Explained:**

| Level | Requirements | Protection |
|-------|------------|------------|
| **SLSA 0** | No provenance | None |
| **SLSA 1** | Build process documented | Awareness |
| **SLSA 2** | Provenance generated & authenticated | Tampering after build |
| **SLSA 3** | Hardened, isolated build service | Build system compromise |
| **SLSA 4** | 2-person review + hermetic builds | Sophisticated attacks |

**Example findings:**
```json
{
  "provenance": {
    "present": true,
    "format": "slsa",
    "slsa_level": 3,
    "builder": "GitHub Actions",
    "build_type": "https://github.com/slsa-framework/slsa-github-generator",
    "source_repo": "github.com/ryan/myapp",
    "source_commit": "a3b7c9d...",
    "verified": true
  }
}
```

---

### 3. Signatures

**What it is:**
Cryptographic signatures proving image authenticity.

**Why it matters:**
- Verify image hasn't been tampered with
- Confirm the publisher's identity
- Enable policy-based admission control
- Transparency via Rekor log

**What we check:**
- ✅ Image is signed
- ✅ Signature verification (Cosign/Sigstore)
- ✅ Rekor transparency log entry
- ✅ Fulcio certificate (keyless signing)
- ✅ Trusted signers

**Scoring:**
- Signed: +15 points
- Cosign verified: +10 points
- **Total: 25 points**

**Signing Methods:**

| Method | Use Case | Security |
|--------|----------|----------|
| **Key-based** | Long-lived keys stored securely | High (if keys protected) |
| **Keyless** | OIDC identity (GitHub, Google, etc.) | High (no key management) |
| **Notary** | Docker Content Trust | Medium (deprecated) |

**Example findings:**
```json
{
  "signatures": {
    "signed": true,
    "signature_count": 2,
    "cosign_verified": true,
    "rekor_entry": "https://rekor.sigstore.dev/api/v1/log/entries/...",
    "fulcio_issuer": "https://github.com/login/oauth",
    "trusted_signers": ["ryan@afterdarksys.com"],
    "signatures": [
      {
        "signer": "ryan@afterdarksys.com",
        "signed_at": "2026-03-12T10:30:00Z",
        "algorithm": "ECDSA-SHA256",
        "verified": true
      }
    ]
  }
}
```

---

### 4. Dependencies

**What it is:**
Analysis of third-party packages and their risks.

**Why it matters:**
- Detect malicious packages (supply chain attacks)
- Identify typosquatting attempts
- Find unmaintained/abandoned packages
- Check license compatibility
- Track security updates

**What we check:**
- ✅ High-risk packages (known malware, suspicious patterns)
- ✅ Typosquatting detection
- ✅ Unmaintained packages
- ✅ License conflicts
- ✅ Available security updates
- ✅ Outdated packages

**Scoring:**
- Low risk factor: up to +15 points
- **Total: 15 points**

**Risk Factors:**

| Factor | Example | Risk Level |
|--------|---------|------------|
| **Known malware** | Package contains crypto miner | CRITICAL |
| **Typosquatting** | `reqeusts` instead of `requests` | HIGH |
| **Unmaintained** | No updates in 3+ years | MEDIUM |
| **License conflict** | GPL mixed with proprietary | MEDIUM |
| **Critical CVE** | Unpatched vulnerability | HIGH |

**Example findings:**
```json
{
  "dependencies": {
    "total_dependencies": 247,
    "direct_dependencies": 34,
    "transitive_dependencies": 213,
    "high_risk_packages": [
      {
        "name": "reqeusts",
        "version": "2.28.0",
        "ecosystem": "pypi",
        "reasons": ["Typosquatting of 'requests'"],
        "risk_score": 95
      }
    ],
    "typosquatting": ["reqeusts", "urlib3"],
    "unmaintained_packages": ["old-parser"],
    "outdated_packages": 12,
    "security_updates_available": 3
  }
}
```

---

## Maturity Levels

Based on the overall score (0-100), images are classified into maturity levels:

| Level | Score | Description |
|-------|-------|-------------|
| **None** | 0-29 | No supply chain security controls |
| **Basic** | 30-49 | Minimal controls, significant gaps |
| **Intermediate** | 50-69 | Some controls in place, needs improvement |
| **Advanced** | 70-89 | Strong security posture, minor gaps |
| **Exemplary** | 90-100 | Industry-leading security practices |

### Score Breakdown

| Component | Max Points | Weight |
|-----------|------------|--------|
| SBOM | 30 | 30% |
| Provenance | 30 | 30% |
| Signatures | 25 | 25% |
| Dependencies | 15 | 15% |
| **Total** | **100** | **100%** |

---

## Compliance Frameworks

### SLSA (Supply-chain Levels for Software Artifacts)

**Purpose:** Prevent tampering, improve integrity, secure packages.

**Our Compliance Check:**

✅ **SLSA Level 1**: Build process is fully scripted/automated
✅ **SLSA Level 2**: Provenance is available and authenticated
✅ **SLSA Level 3**: Build service is hardened and isolated
✅ **SLSA Level 4**: 2-person review + hermetic, reproducible builds

**Example output:**
```json
{
  "slsa": {
    "level": 3,
    "requirements": [
      "Build process is fully scripted/automated",
      "Provenance is available and authenticated",
      "Build service is hardened and isolated"
    ],
    "missing": [
      "Two-person review of all changes",
      "Hermetic, reproducible builds"
    ]
  }
}
```

---

### NIST SSDF (Secure Software Development Framework)

**Purpose:** Mitigate risk of software vulnerabilities.

**Our Compliance Check:**

We evaluate 4 key practices:

1. **PO.3**: Obtain software components from trusted sources
   - ✅ Met if SBOM present
2. **PS.2**: Create and maintain secure development environment
   - ✅ Met if provenance present
3. **PW.4**: Create and maintain integrity verification mechanism
   - ✅ Met if image is signed
4. **RV.1**: Identify and confirm vulnerabilities
   - ✅ Met if no high-risk packages

**Example output:**
```json
{
  "ssdf": {
    "compliant": true,
    "practices_met": [
      "PO.3: Obtain software components from trusted sources",
      "PS.2: Create and maintain development environment",
      "PW.4: Create integrity verification mechanism",
      "RV.1: Identify and confirm vulnerabilities"
    ],
    "gaps_practices": []
  }
}
```

---

### CIS Docker Benchmark

**Purpose:** Security configuration best practices for Docker.

**Our Compliance Check:**

We evaluate container build and runtime security:
- ✅ No privileged containers
- ✅ Read-only root filesystem
- ✅ Resource limits set
- ✅ No hardcoded secrets
- ✅ Minimal base image

**Example output:**
```json
{
  "cis": {
    "level": 2,
    "passed_checks": 87,
    "failed_checks": 3,
    "score": 96
  }
}
```

---

## Using the Analyzer

### Via API

```bash
curl -X POST https://apps.afterdarksys.com:5005/api/v2/supply-chain/analyze \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "image_digest": "sha256:abc123..."
  }'
```

### Via Starlark Script

```python
analysis = scan.analyze_supply_chain("sha256:abc123...")
print(f"Security Score: {analysis['overall_score']}/100")
print(f"Maturity Level: {analysis['maturity_level']}")

for gap in analysis['gaps']:
    print(f"[{gap['severity']}] {gap['description']}")
    print(f"  Remediation: {gap['remediation']}")
```

### Via CLI

```bash
ads-registry supply-chain analyze myapp/frontend:latest
```

---

## Remediation Guide

### Gap: No SBOM

**Severity:** HIGH

**Impact:** Cannot track components, licenses, or respond to vulnerabilities.

**Remediation:**

**Option 1: Syft (recommended)**
```bash
# Install Syft
curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh

# Generate SBOM
syft myapp/frontend:latest -o cyclonedx-json > sbom.json

# Attach to image
oras attach apps.afterdarksys.com:5005/myapp/frontend:latest \
  --artifact-type application/vnd.cyclonedx+json \
  sbom.json
```

**Option 2: Docker buildx**
```dockerfile
# In Dockerfile
FROM alpine:latest
# ... your build steps ...

# Generate SBOM at build time
RUN apk add --no-cache syft && \
    syft / -o spdx-json > /sbom.spdx.json
```

**Option 3: CI/CD Integration**
```yaml
# GitHub Actions
- name: Generate SBOM
  uses: anchore/sbom-action@v0
  with:
    image: myapp/frontend:latest
    format: cyclonedx-json
```

---

### Gap: No Provenance

**Severity:** CRITICAL

**Impact:** Cannot verify build integrity or source authenticity.

**Remediation:**

**Option 1: SLSA GitHub Generator (recommended)**
```yaml
# .github/workflows/build.yml
name: Build
on: push

permissions:
  id-token: write  # For keyless signing
  contents: read
  packages: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Build and push with provenance
        uses: docker/build-push-action@v4
        with:
          context: .
          push: true
          tags: apps.afterdarksys.com:5005/myapp/frontend:${{ github.sha }}
          provenance: true  # ← Generates SLSA provenance
```

**Option 2: GitLab SLSA**
```yaml
# .gitlab-ci.yml
build:
  image: docker:latest
  script:
    - docker build -t $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA .
    - docker push $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
  artifacts:
    attestations:
      - type: slsa_provenance_v0.2
```

**Option 3: Manual with in-toto**
```bash
# Install in-toto
pip install in-toto

# Record build steps
in-toto-run \
  --step-name build \
  --key build.key \
  --materials . \
  --products dist/ \
  -- docker build -t myapp:latest .

# Upload provenance
curl -X POST https://apps.afterdarksys.com:5005/api/v2/attestations \
  -H "Content-Type: application/json" \
  -d @build.provenance.json
```

---

### Gap: Not Signed

**Severity:** HIGH

**Impact:** Cannot verify image authenticity or detect tampering.

**Remediation:**

**Option 1: Keyless Signing with Cosign (recommended)**
```bash
# Install Cosign
curl -Lo cosign https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64
chmod +x cosign

# Sign using OIDC (no key management!)
cosign sign \
  --oidc-issuer=https://github.com/login/oauth \
  apps.afterdarksys.com:5005/myapp/frontend:latest

# Verify
cosign verify \
  --certificate-identity=ryan@afterdarksys.com \
  --certificate-oidc-issuer=https://github.com/login/oauth \
  apps.afterdarksys.com:5005/myapp/frontend:latest
```

**Option 2: Key-based Signing**
```bash
# Generate key pair
cosign generate-key-pair

# Sign
cosign sign \
  --key cosign.key \
  apps.afterdarksys.com:5005/myapp/frontend:latest

# Verify
cosign verify \
  --key cosign.pub \
  apps.afterdarksys.com:5005/myapp/frontend:latest
```

**Option 3: CI/CD Integration**
```yaml
# GitHub Actions
- name: Sign image
  run: |
    cosign sign \
      --oidc-issuer=${{ env.OIDC_ISSUER }} \
      apps.afterdarksys.com:5005/myapp/frontend:${{ github.sha }}
  env:
    COSIGN_EXPERIMENTAL: 1
```

---

### Gap: High-Risk Dependencies

**Severity:** HIGH

**Impact:** Potential security vulnerabilities or supply chain attacks.

**Remediation:**

1. **Remove typosquatted packages**
   ```bash
   # Bad: reqeusts
   # Good: requests
   pip uninstall reqeusts
   pip install requests
   ```

2. **Update packages with security fixes**
   ```bash
   npm audit fix
   pip install --upgrade package-name
   ```

3. **Replace unmaintained packages**
   - Research alternatives
   - Fork and maintain internally if critical

4. **Use dependency scanning in CI/CD**
   ```yaml
   # GitHub Actions
   - name: Dependency scan
     uses: aquasecurity/trivy-action@master
     with:
       scan-type: 'fs'
       scan-ref: 'package.json'
   ```

---

## Best Practices

### 1. Start Early

Integrate supply chain security from day 1:

```
Code → Build → SBOM → Sign → Provenance → Push → Scan → Deploy
       ↑                                    ↑
       ├─ Generate SBOM                    ├─ Verify before deploy
       └─ Record provenance                └─ Policy enforcement
```

### 2. Automate Everything

Manual processes don't scale:

✅ Generate SBOM automatically during build
✅ Sign images in CI/CD pipeline
✅ Generate provenance from build metadata
✅ Scan before pushing to registry

### 3. Enforce Policies

Use admission controllers:

```yaml
# Kubernetes admission policy
apiVersion: policy.sigstore.dev/v1beta1
kind: ClusterImagePolicy
metadata:
  name: require-signatures
spec:
  images:
  - glob: "apps.afterdarksys.com:5005/**"
  authorities:
  - keyless:
      identities:
      - issuer: https://github.com/login/oauth
        subject: ryan@afterdarksys.com
```

### 4. Monitor and Alert

Track your supply chain security posture:

- Weekly supply chain reports
- Alerts on new vulnerabilities
- Dashboard showing maturity levels
- Track improvement over time

### 5. Continuous Improvement

| Current Level | Next Step |
|--------------|-----------|
| **None** | Generate SBOMs |
| **Basic** | Add signatures |
| **Intermediate** | Implement SLSA Level 2 |
| **Advanced** | Achieve SLSA Level 3 |
| **Exemplary** | Aim for SLSA Level 4 |

---

## Resources

### Tools

- **SBOM**: [Syft](https://github.com/anchore/syft), [CycloneDX](https://cyclonedx.org/)
- **Signing**: [Cosign](https://github.com/sigstore/cosign), [Sigstore](https://sigstore.dev/)
- **Provenance**: [SLSA](https://slsa.dev/), [in-toto](https://in-toto.io/)
- **Scanning**: [Trivy](https://trivy.dev/), [Grype](https://github.com/anchore/grype)

### Frameworks

- **SLSA**: https://slsa.dev/
- **NIST SSDF**: https://csrc.nist.gov/Projects/ssdf
- **CIS Benchmarks**: https://www.cisecurity.org/benchmark/docker
- **CNCF Security**: https://www.cncf.io/blog/tag/security/

### Learning

- **Supply Chain Security 101**: https://slsa.dev/blog/2023/05/supply-chain-security-101
- **Sigstore Tutorial**: https://docs.sigstore.dev/
- **SBOM Guide**: https://www.cisa.gov/sbom

---

**Secure your software supply chain today!** 🔒
