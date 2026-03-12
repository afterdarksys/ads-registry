# Static Analysis Scanner (Semgrep Integration)

**Automated code quality and security scanning for container images**

---

## Overview

The Static Analysis Scanner automatically scans container images for:

- 🔐 **Hardcoded Secrets** - API keys, passwords, private keys, AWS credentials
- 🛡️ **Security Vulnerabilities** - SQL injection, XSS, insecure configurations (CWE/OWASP)
- 📊 **Code Quality Issues** - Complexity, duplication, maintainability problems
- 🐳 **Dockerfile Best Practices** - Root user, latest tags, missing health checks

---

## How It Works

```
Image Push → Extract Layers → Static Analysis → Report Generated → Owner Notified
     ↓              ↓                 ↓                ↓                  ↓
  Registry     Filesystem       Semgrep + Regex    Save to DB    Email/Slack/Webhook
               Extraction       Pattern Matching   Findings      if severity met
```

**Analysis Workflow:**

1. **Image pushed** to registry → Scan queued
2. **Layers extracted** to temporary directory
3. **Multiple scans run in parallel**:
   - Secret detection (regex + entropy analysis)
   - Semgrep security rules
   - Code smell detection
   - Dockerfile analysis
4. **Findings aggregated** and saved to database
5. **Notifications sent** if severity threshold met

---

## What It Detects

### 1. Hardcoded Secrets

Detects sensitive credentials hardcoded in source code:

| Type | Pattern Example | Severity |
|------|----------------|----------|
| AWS Access Keys | `AKIA[0-9A-Z]{16}` | CRITICAL |
| GitHub Tokens | `ghp_[0-9a-zA-Z]{36}` | CRITICAL |
| Private Keys | `-----BEGIN RSA PRIVATE KEY-----` | CRITICAL |
| Database URLs | `postgresql://user:pass@host/db` | CRITICAL |
| API Keys | High entropy strings in variables | HIGH |
| Passwords | `password = "..."` patterns | HIGH |
| JWT Tokens | `eyJ...` (3-part base64) | HIGH |
| Slack Webhooks | `hooks.slack.com/services/...` | HIGH |

**Detection Methods:**
- **Regex Patterns**: Known secret formats
- **Shannon Entropy**: Detects high-randomness strings (potential keys)
- **Context Analysis**: Variable names like `api_key`, `password`, `secret`
- **False Positive Filtering**: Excludes `example`, `test`, `dummy`, `TODO`

### 2. Security Vulnerabilities

Detects security issues using Semgrep rules:

| Category | Examples | Standards |
|----------|----------|-----------|
| Injection | SQL injection, command injection, XSS | CWE-89, CWE-78, CWE-79 |
| Authentication | Weak crypto, missing auth checks | CWE-327, CWE-306 |
| Authorization | Insecure permissions, path traversal | CWE-22, CWE-862 |
| Cryptography | Hardcoded IV, weak algorithms | CWE-321, CWE-326 |
| OWASP Top 10 | All OWASP categories mapped | A01-A10 |

**Semgrep Rulesets:**
- `auto` mode: Community rules + security focused
- Custom rules: `/etc/semgrep/rules/`

### 3. Code Quality Issues

Identifies maintainability problems:

| Category | Examples |
|----------|----------|
| Complexity | Cyclomatic complexity > 10, deep nesting |
| Duplication | Copy-pasted code blocks |
| Maintainability | Long functions (>100 lines), god objects |
| Dead Code | Unreachable code, unused variables |

### 4. Dockerfile Analysis

Checks Dockerfile for best practices:

| Check | Deduction | Issue |
|-------|-----------|-------|
| Root User | -20 points | Running as `USER root` (security risk) |
| Secrets in ENV | -30 points | `ENV PASSWORD=...` (credentials exposed) |
| Latest Tag | -10 points | `FROM ubuntu:latest` (not reproducible) |
| Missing HEALTHCHECK | -15 points | No `HEALTHCHECK` instruction |

**Scoring:** Starts at 100, deducts points for issues. Final score: 0-100.

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SEMGREP_PATH` | Auto-detected | Path to Semgrep binary |
| `SEMGREP_RULES_PATH` | `/etc/semgrep/rules` | Custom rules directory |
| `STATIC_ANALYSIS_TEMP_DIR` | `/tmp/static-analysis` | Temporary extraction directory |

### Install Semgrep

```bash
# pip
pip install semgrep

# Docker
docker pull returntocorp/semgrep

# Homebrew (macOS)
brew install semgrep

# Verify installation
semgrep --version
```

### Custom Rules

Add custom Semgrep rules to `/etc/semgrep/rules/`:

```yaml
# /etc/semgrep/rules/custom-secrets.yaml
rules:
  - id: internal-api-key-pattern
    pattern: internal_api_key = "$VALUE"
    message: Internal API key detected
    severity: WARNING
    languages: [python, javascript]
```

---

## Scan Reports

### Report Structure

```json
{
  "digest": "sha256:abc123...",
  "scan_time": "2026-03-12T10:30:00Z",
  "secrets": [
    {
      "type": "aws_access_key",
      "file_path": "config/aws.py",
      "line_number": 42,
      "snippet": "AKIA****1234",
      "entropy": 4.8,
      "confidence": "high",
      "severity": "critical"
    }
  ],
  "code_smells": [
    {
      "rule_id": "complexity.cyclomatic-too-high",
      "category": "complexity",
      "file_path": "handlers/auth.go",
      "line_number": 156,
      "message": "Function has cyclomatic complexity of 18 (max 10)",
      "severity": "medium",
      "suggestion": "Refactor into smaller functions"
    }
  ],
  "security": [
    {
      "rule_id": "python.lang.security.audit.sql-injection",
      "cwe": "CWE-89",
      "owasp": "A01:2021-Injection",
      "file_path": "models/user.py",
      "line_number": 89,
      "description": "SQL query constructed using string concatenation",
      "severity": "high",
      "fix": "Use parameterized queries or ORM"
    }
  ],
  "dockerfile": {
    "has_root_user": true,
    "has_secrets_in_env": false,
    "uses_latest_tag": true,
    "missing_health_check": true,
    "issues": [
      "Running as root user",
      "Using :latest tag (not reproducible)",
      "Missing HEALTHCHECK instruction"
    ],
    "score": 55
  },
  "summary": {
    "total_findings": 47,
    "critical_secrets": 3,
    "high_severity": 12,
    "medium_severity": 20,
    "low_severity": 12,
    "files_scanned": 234,
    "lines_of_code_scanned": 15678
  }
}
```

### View Scan Results

**Via API:**

```bash
# Get detailed report
curl https://apps.afterdarksys.com:5005/api/v2/scans/sha256:abc123/static-analysis

# Get all scan results for an image
curl https://apps.afterdarksys.com:5005/api/v2/scans/sha256:abc123
```

**Via Database:**

```sql
-- Get latest static analysis report
SELECT * FROM security_scan_jobs
WHERE image_digest = 'sha256:abc123...'
AND scanner = 'semgrep-static-analysis'
ORDER BY created_at DESC
LIMIT 1;

-- Get all critical secrets found
SELECT * FROM vulnerability_findings
WHERE image_digest = 'sha256:abc123...'
AND severity = 'CRITICAL'
AND type = 'SECRET';
```

---

## Notifications

Owners are automatically notified when static analysis findings exceed severity thresholds.

### Configure Notification Preferences

```sql
-- Only notify about critical secrets and high security issues
UPDATE security_notification_preferences
SET cve_threshold = 'HIGH',
    email_enabled = true,
    slack_enabled = true
WHERE user_id = 123;
```

### Example Notification

**Email:**
```
Subject: [Security Alert] Static analysis found 3 critical secrets in image abc123

Static Analysis Results

Image Digest: sha256:abc123...
Scan Time: 2026-03-12T10:30:00Z

Critical Findings:
  - 2 AWS Access Keys in config/aws.py
  - 1 GitHub Token in .github/workflows/deploy.yml

Security Issues:
  - 5 SQL injection vulnerabilities
  - 3 XSS vulnerabilities

Dockerfile Score: 55/100
  - Running as root user
  - Using :latest tags
  - Missing HEALTHCHECK
```

**Slack:**
```
🔒 Static Analysis Completed

Image: abc123...

Critical Secrets: 3
High Severity: 12
Dockerfile Score: 55/100

View Report: https://apps.afterdarksys.com:5005/scans/abc123
```

---

## Remediation Guide

### Fix Hardcoded Secrets

**Before:**
```python
# ❌ BAD: Hardcoded AWS credentials
aws_access_key = "AKIAIOSFODNN7EXAMPLE"
aws_secret_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

client = boto3.client(
    's3',
    aws_access_key_id=aws_access_key,
    aws_secret_access_key=aws_secret_key
)
```

**After:**
```python
# ✅ GOOD: Use environment variables or secret management
import os

client = boto3.client('s3')  # Uses AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY env vars

# Or use registry vault integration:
from ads_registry_client import get_secret
client = boto3.client(
    's3',
    aws_access_key_id=get_secret("aws/access_key_id"),
    aws_secret_access_key=get_secret("aws/secret_access_key")
)
```

### Fix SQL Injection

**Before:**
```python
# ❌ BAD: SQL injection vulnerability
def get_user(username):
    query = f"SELECT * FROM users WHERE username = '{username}'"
    return db.execute(query)
```

**After:**
```python
# ✅ GOOD: Parameterized query
def get_user(username):
    query = "SELECT * FROM users WHERE username = ?"
    return db.execute(query, (username,))
```

### Fix Dockerfile Issues

**Before:**
```dockerfile
# ❌ BAD: Multiple issues
FROM ubuntu:latest
RUN apt-get update && apt-get install -y python3
ENV DATABASE_PASSWORD=my-secret-password
USER root
CMD ["python3", "app.py"]
```

**After:**
```dockerfile
# ✅ GOOD: Security best practices
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y python3 && \
    rm -rf /var/lib/apt/lists/*

# Use secret management
ENV DATABASE_URL_FILE=/run/secrets/db_url

# Create non-root user
RUN useradd -m -u 1000 appuser
USER appuser

# Add health check
HEALTHCHECK --interval=30s --timeout=3s \
  CMD curl -f http://localhost:8080/health || exit 1

CMD ["python3", "app.py"]
```

---

## Integration

### CI/CD Pipeline

**GitHub Actions:**

```yaml
name: Security Scan
on: push

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - name: Build and push image
        run: |
          docker build -t apps.afterdarksys.com:5005/myapp:${{ github.sha }} .
          docker push apps.afterdarksys.com:5005/myapp:${{ github.sha }}

      - name: Wait for scan
        run: sleep 30

      - name: Check scan results
        run: |
          RESULTS=$(curl https://apps.afterdarksys.com:5005/api/v2/scans/sha256:$DIGEST/static-analysis)
          CRITICAL=$(echo $RESULTS | jq '.summary.critical_secrets')

          if [ "$CRITICAL" -gt 0 ]; then
            echo "❌ CRITICAL: Found $CRITICAL hardcoded secrets!"
            exit 1
          fi
```

### Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Scan for secrets before commit
semgrep --config auto --severity ERROR .

if [ $? -ne 0 ]; then
  echo "❌ Pre-commit check failed: secrets or security issues detected"
  exit 1
fi
```

---

## Performance

### Scan Times

| Image Size | Files | Lines of Code | Scan Time |
|------------|-------|---------------|-----------|
| Small (50MB) | 100 | 5,000 | ~10 seconds |
| Medium (500MB) | 1,000 | 50,000 | ~45 seconds |
| Large (2GB) | 5,000 | 250,000 | ~3 minutes |

### Optimization

**Skip large files:**
```bash
# Configure in Semgrep
export SEMGREP_TIMEOUT=30
export SEMGREP_MAX_MEMORY_MB=2000
```

**Exclude patterns:**
Create `.semgrepignore`:
```
node_modules/
*.min.js
vendor/
third_party/
```

---

## Troubleshooting

### Semgrep Not Found

**Error:**
```
[StaticAnalyzer] Semgrep not found
```

**Fix:**
```bash
# Install Semgrep
pip install semgrep

# Or specify path
export SEMGREP_PATH=/usr/local/bin/semgrep
```

### Scan Taking Too Long

**Error:**
```
[StaticAnalyzer] Scan timeout after 5 minutes
```

**Fix:**
```bash
# Increase timeout
export SEMGREP_TIMEOUT=600  # 10 minutes

# Or exclude large directories
echo "node_modules/" >> .semgrepignore
```

### False Positives

**Problem:** Detecting test/example secrets as real secrets

**Fix:**

1. **Use .semgrepignore:**
   ```
   tests/
   examples/
   *_test.go
   ```

2. **Mark as false positive in code:**
   ```python
   # nosemgrep: python.lang.security.audit.hardcoded-password
   test_password = "test123"
   ```

### Extraction Failed

**Error:**
```
[StaticAnalyzer] Failed to extract image: tar extraction failed
```

**Fix:**
```bash
# Check disk space
df -h /tmp

# Clean old scans
rm -rf /tmp/static-analysis/*
```

---

## Best Practices

### 1. Zero Secrets in Code

Never commit secrets. Use:
- Environment variables
- Secret management (Vault, AWS Secrets Manager)
- Registry built-in secret storage

### 2. Regular Scans

```bash
# Schedule periodic re-scans (cron)
0 2 * * * curl -X POST https://apps.afterdarksys.com:5005/api/v2/scans/rescan-all
```

### 3. Block Deployments

Fail CI/CD if critical issues found:

```yaml
# Block deployment if critical secrets detected
- name: Check critical findings
  run: |
    CRITICAL=$(jq '.summary.critical_secrets' scan-results.json)
    if [ "$CRITICAL" -gt 0 ]; then
      exit 1
    fi
```

### 4. Dockerfile Standards

Enforce Dockerfile score minimum:

```bash
# Require 80+ score
SCORE=$(jq '.dockerfile.score' scan-results.json)
if [ "$SCORE" -lt 80 ]; then
  echo "❌ Dockerfile score too low: $SCORE/100"
  exit 1
fi
```

### 5. Custom Rules

Create organization-specific rules:

```yaml
# /etc/semgrep/rules/org-secrets.yaml
rules:
  - id: internal-api-pattern
    pattern: |
      INTERNAL_API_KEY = "..."
    message: Internal API key should use secret manager
    severity: ERROR
```

---

## API Reference

### Trigger Manual Scan

```bash
POST /api/v2/scans/trigger

{
  "digest": "sha256:abc123...",
  "scanner": "semgrep-static-analysis"
}
```

### Get Scan Results

```bash
GET /api/v2/scans/{digest}/static-analysis

Response: StaticAnalysisReport (JSON)
```

### Delete Old Scans

```bash
DELETE /api/v2/scans/{digest}?scanner=semgrep-static-analysis&older_than=30d
```

---

## Architecture

### Scanner Components

```
StaticAnalyzer
├── SecretDetectors[]         # Pattern-based secret detection
│   ├── AWS Access Key Detector
│   ├── GitHub Token Detector
│   ├── Private Key Detector
│   └── Generic High-Entropy Detector
├── Semgrep Integration        # Code quality & security
│   ├── Execute semgrep --config auto
│   ├── Parse JSON results
│   └── Map to CWE/OWASP
├── Dockerfile Analyzer        # Dockerfile best practices
│   ├── Check for root user
│   ├── Detect secrets in ENV
│   ├── Check for :latest tags
│   └── Verify HEALTHCHECK
└── Report Generator           # Aggregate findings
    ├── Calculate summary stats
    ├── Assign severity levels
    └── Generate notifications
```

### Database Schema

```sql
-- Static analysis findings
CREATE TABLE static_analysis_findings (
    id SERIAL PRIMARY KEY,
    image_digest VARCHAR(255),
    scan_time TIMESTAMP,
    finding_type VARCHAR(50),  -- secret, security, code_smell, dockerfile
    severity VARCHAR(20),      -- critical, high, medium, low
    file_path TEXT,
    line_number INTEGER,
    rule_id VARCHAR(100),
    message TEXT,
    fix_suggestion TEXT,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_static_findings_digest ON static_analysis_findings(image_digest);
CREATE INDEX idx_static_findings_severity ON static_analysis_findings(severity);
```

---

## Comparison with Other Scanners

| Scanner | CVE Detection | Secrets | Code Quality | Dockerfile | Custom Rules |
|---------|---------------|---------|--------------|------------|--------------|
| **Semgrep (This)** | ✅ Patterns | ✅ Yes | ✅ Yes | ✅ Yes | ✅ YAML |
| **Trivy** | ✅ CVE DB | ✅ Yes | ❌ No | ✅ Limited | ❌ No |
| **Snyk** | ✅ CVE DB | ✅ Yes | ❌ No | ✅ Yes | ❌ Paid |
| **Grype** | ✅ CVE DB | ❌ No | ❌ No | ❌ No | ❌ No |

**When to use Static Analysis:**
- ✅ Detect secrets before production
- ✅ Enforce code quality standards
- ✅ Find security patterns (SQL injection, XSS)
- ✅ Dockerfile best practices

**When to use CVE scanners (Trivy):**
- ✅ Known vulnerability databases
- ✅ Package version tracking
- ✅ SBOM generation

**Best Practice:** Use both! They complement each other.

---

## References

- [Semgrep Documentation](https://semgrep.dev/docs/)
- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [CWE List](https://cwe.mitre.org/data/index.html)
- [Dockerfile Best Practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/)

---

**Questions?** Check [GitHub Discussions](https://github.com/ryan/ads-registry/discussions) or file an [issue](https://github.com/ryan/ads-registry/issues).
