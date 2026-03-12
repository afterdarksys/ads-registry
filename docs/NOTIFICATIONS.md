# Security Scan Notifications

**Automatic notifications when vulnerabilities are found in your images**

---

## Overview

The ADS Container Registry automatically notifies image owners when security scans detect vulnerabilities. Notifications are sent via:

- ✉️ **Email** - Detailed vulnerability reports
- 🔔 **Webhooks** - JSON payloads to your services
- 💬 **Slack** - Real-time alerts in your channels

---

## How It Works

```
Image Push → Scanner Detects CVEs → Notification Service → Owner Notified
     ↓              ↓                        ↓                    ↓
  Registry     Trivy/ClamAV          Checks preferences      Email/Slack/Webhook
               Semgrep/Falco         Gets owner info         Sends alert
```

**Automatic Workflow:**

1. **Image pushed** to registry → Scan queued
2. **Scanner runs** (Trivy, ClamAV, Semgrep, etc.)
3. **Results saved** to database
4. **Owner identified** via repository ownership
5. **Preferences checked** (severity threshold, channels)
6. **Notifications sent** if thresholds met

---

## Notification Preferences

### Database Schema

```sql
CREATE TABLE security_notification_preferences (
    user_id INTEGER PRIMARY KEY,

    -- Channels
    email_enabled BOOLEAN DEFAULT true,
    webhook_enabled BOOLEAN DEFAULT false,
    webhook_url VARCHAR(500),
    slack_enabled BOOLEAN DEFAULT false,
    slack_webhook VARCHAR(500),

    -- Thresholds
    cve_threshold VARCHAR(20) DEFAULT 'HIGH',  -- CRITICAL, HIGH, MEDIUM, LOW

    -- Frequency
    immediate_notification BOOLEAN DEFAULT true,
    daily_digest BOOLEAN DEFAULT false,
    weekly_digest BOOLEAN DEFAULT false,

    -- Filters
    only_my_images BOOLEAN DEFAULT true,
    include_group_images BOOLEAN DEFAULT false
);
```

### Setting Preferences

**Via API:**

```bash
curl -X PUT https://apps.afterdarksys.com:5005/api/v2/users/me/notification-preferences \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "email_enabled": true,
    "cve_threshold": "HIGH",
    "immediate_notification": true,
    "slack_enabled": true,
    "slack_webhook": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
  }'
```

**Via CLI:**

```bash
ads-registry user set-notifications \
  --email=true \
  --threshold=HIGH \
  --slack-webhook=https://hooks.slack.com/services/...
```

**Via SQL:**

```sql
INSERT INTO security_notification_preferences (
    user_id, email_enabled, cve_threshold, slack_enabled, slack_webhook
)
VALUES (
    123, true, 'HIGH', true, 'https://hooks.slack.com/services/...'
)
ON CONFLICT (user_id) DO UPDATE SET
    email_enabled = EXCLUDED.email_enabled,
    cve_threshold = EXCLUDED.cve_threshold,
    slack_enabled = EXCLUDED.slack_enabled,
    slack_webhook = EXCLUDED.slack_webhook;
```

---

## Notification Channels

### 1. Email Notifications

**Configuration:**

```bash
# Environment variables
export SMTP_HOST=smtp.gmail.com
export SMTP_PORT=587
export SMTP_USERNAME=registry@afterdarksys.com
export SMTP_PASSWORD=your-app-password
export SMTP_FROM=registry@afterdarksys.com
```

**Example Email:**

```
Subject: [Security Alert] 12 vulnerabilities found in image a3b7c9d1...

Security Scan Results

Image Digest: sha256:a3b7c9d1e4f5...
Scan Time: 2026-03-12T10:30:00Z

Vulnerability Summary:
  CRITICAL: 2
  HIGH:     10
  MEDIUM:   15
  LOW:      5

Top Vulnerabilities:

1. CVE-2024-1234 [CRITICAL] in openssl 1.1.1
   Fixed in: 1.1.1t

2. CVE-2024-5678 [CRITICAL] in curl 7.68.0
   Fixed in: 7.88.1

3. CVE-2024-9012 [HIGH] in nginx 1.18.0
   Fixed in: 1.24.0

---
ADS Container Registry Security Notifications
```

---

### 2. Webhook Notifications

**Payload Format:**

```json
{
  "event": "image.scan.completed",
  "digest": "sha256:a3b7c9d1e4f5...",
  "timestamp": "2026-03-12T10:30:00Z",
  "total_vulns": 32,
  "critical": 2,
  "high": 10,
  "medium": 15,
  "low": 5,
  "top_cves": [
    {
      "id": "CVE-2024-1234",
      "package": "openssl",
      "version": "1.1.1",
      "fix_version": "1.1.1t",
      "severity": "CRITICAL",
      "description": "Buffer overflow in OpenSSL..."
    }
  ]
}
```

**Example Handler (Node.js):**

```javascript
const express = require('express');
const app = express();

app.post('/registry-webhook', express.json(), (req, res) => {
  const { event, digest, critical, high } = req.body;

  if (event === 'image.scan.completed') {
    if (critical > 0 || high > 5) {
      // Create Jira ticket
      createJiraTicket({
        summary: `Security alert for image ${digest.slice(0, 12)}`,
        description: `Found ${critical} critical and ${high} high severity CVEs`,
        priority: critical > 0 ? 'Highest' : 'High'
      });
    }
  }

  res.json({ received: true });
});

app.listen(8080);
```

---

### 3. Slack Notifications

**Setup:**

1. Create Slack Incoming Webhook:
   - Go to https://api.slack.com/messaging/webhooks
   - Create webhook for your channel
   - Copy webhook URL

2. Configure in registry:
   ```sql
   UPDATE security_notification_preferences
   SET slack_enabled = true,
       slack_webhook = 'https://hooks.slack.com/services/T00/B00/XXX'
   WHERE user_id = 123;
   ```

**Example Slack Message:**

```
🔒 Security Scan Completed

Image: a3b7c9d1e4f5

Total Vulnerabilities: 32
Critical: 2
High: 10
Medium: 15
```

**Color Coding:**
- 🔴 Red: Critical vulnerabilities found
- 🟡 Yellow: High vulnerabilities found
- 🟢 Green: No critical/high vulnerabilities

---

## Severity Thresholds

Control when you get notified:

| Threshold | You're Notified When |
|-----------|---------------------|
| `CRITICAL` | Any CRITICAL CVE found |
| `HIGH` | Any CRITICAL or HIGH CVE found |
| `MEDIUM` | Any CRITICAL, HIGH, or MEDIUM CVE found |
| `LOW` | Any vulnerability found |

**Example:**

```sql
-- Only notify me about critical issues
UPDATE security_notification_preferences
SET cve_threshold = 'CRITICAL'
WHERE user_id = 123;

-- Notify me about everything
UPDATE security_notification_preferences
SET cve_threshold = 'LOW'
WHERE user_id = 123;
```

---

## Notification Frequency

### Immediate Notifications

Sent as soon as scan completes (default):

```sql
UPDATE security_notification_preferences
SET immediate_notification = true
WHERE user_id = 123;
```

### Daily Digest

One email per day with all scan results:

```sql
UPDATE security_notification_preferences
SET immediate_notification = false,
    daily_digest = true
WHERE user_id = 123;
```

**Daily Digest Format:**

```
Daily Security Digest - March 12, 2026

Images Scanned Today: 15

High Priority Issues:
  - myapp/frontend:v2.1 - 3 CRITICAL CVEs
  - myapp/backend:v1.9 - 8 HIGH CVEs

Clean Images:
  - myapp/database:latest - No vulnerabilities
  - myapp/cache:v3.0 - No vulnerabilities
```

### Weekly Digest

One email per week with summary:

```sql
UPDATE security_notification_preferences
SET immediate_notification = false,
    weekly_digest = true
WHERE user_id = 123;
```

---

## Filtering

### Only My Images

Only notify about images you own (default):

```sql
UPDATE security_notification_preferences
SET only_my_images = true
WHERE user_id = 123;
```

### Include Group Images

Also notify about images owned by your groups:

```sql
UPDATE security_notification_preferences
SET include_group_images = true
WHERE user_id = 123;
```

---

## Audit Trail

All notifications are logged:

```sql
SELECT * FROM security_audit_log
WHERE action = 'notification_sent'
AND user_id = 123
ORDER BY created_at DESC
LIMIT 10;
```

**Example Output:**

| created_at | action | target_type | target_id | metadata |
|------------|--------|-------------|-----------|----------|
| 2026-03-12 10:30:00 | notification_sent | scan | sha256:abc... | {"notification_type": "scan_results", "vulnerability_count": 12} |
| 2026-03-12 09:15:00 | notification_sent | scan | sha256:def... | {"notification_type": "scan_results", "vulnerability_count": 3} |

---

## Testing Notifications

### Send Test Notification

```bash
curl -X POST https://apps.afterdarksys.com:5005/api/v2/notifications/test \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "channel": "email",
    "message": "This is a test notification from ADS Registry"
  }'
```

### Trigger Manual Scan

```bash
# Scan an image and force notification
ads-registry scan --image=myapp/frontend:latest --notify=true
```

---

## Troubleshooting

### Notifications Not Received

1. **Check preferences:**
   ```sql
   SELECT * FROM security_notification_preferences WHERE user_id = YOUR_USER_ID;
   ```

2. **Check if you're the owner:**
   ```sql
   SELECT r.name, r.owner_id
   FROM repositories r
   JOIN manifests m ON r.id = m.repository_id
   WHERE m.digest = 'YOUR_IMAGE_DIGEST';
   ```

3. **Check audit log:**
   ```sql
   SELECT * FROM security_audit_log
   WHERE user_id = YOUR_USER_ID
   AND action = 'notification_sent'
   ORDER BY created_at DESC
   LIMIT 10;
   ```

4. **Check SMTP configuration:**
   ```bash
   echo "SMTP_HOST: $SMTP_HOST"
   echo "SMTP_PORT: $SMTP_PORT"
   ```

### Email Not Sending

- Verify SMTP credentials
- Check firewall rules (port 587/465)
- Test SMTP connection:
  ```bash
  telnet smtp.gmail.com 587
  ```

### Webhook Failing

- Check webhook URL is accessible
- Verify webhook endpoint returns 200 OK
- Check logs:
  ```bash
  grep "Failed to send webhook" /var/log/ads-registry/scanner.log
  ```

### Slack Not Working

- Verify webhook URL is correct
- Test webhook manually:
  ```bash
  curl -X POST https://hooks.slack.com/services/YOUR/WEBHOOK/URL \
    -H 'Content-Type: application/json' \
    -d '{"text": "Test from ADS Registry"}'
  ```

---

## Best Practices

1. **Set appropriate thresholds**
   - Production: `CRITICAL` or `HIGH`
   - Staging: `MEDIUM`
   - Development: `LOW` or disabled

2. **Use webhooks for automation**
   - Create Jira tickets automatically
   - Block deployments with critical CVEs
   - Trigger re-scans after remediation

3. **Configure Slack for teams**
   - Use different channels for different severity levels
   - #security-critical for CRITICAL
   - #security-alerts for HIGH/MEDIUM

4. **Set up daily digests for non-critical**
   - Immediate for CRITICAL/HIGH
   - Daily digest for MEDIUM/LOW

5. **Test notifications regularly**
   - Monthly test to verify email/Slack/webhooks work
   - Update contact information as team changes

---

## Examples

### Example 1: DevOps Team Setup

```sql
-- Team lead gets all notifications immediately
INSERT INTO security_notification_preferences (
    user_id, email_enabled, cve_threshold, immediate_notification,
    slack_enabled, slack_webhook
)
VALUES (
    101, true, 'HIGH', true,
    true, 'https://hooks.slack.com/services/TEAM/CHANNEL/devops-alerts'
);

-- Developers get daily digest
INSERT INTO security_notification_preferences (
    user_id, email_enabled, daily_digest, cve_threshold
)
VALUES
    (102, true, true, 'MEDIUM'),
    (103, true, true, 'MEDIUM'),
    (104, true, true, 'MEDIUM');
```

### Example 2: CI/CD Integration

```yaml
# .github/workflows/deploy.yml
name: Deploy
on: push

jobs:
  security-check:
    runs-on: ubuntu-latest
    steps:
      - name: Check for critical CVEs
        run: |
          SCAN_RESULTS=$(curl https://apps.afterdarksys.com:5005/api/v2/scan-results/$DIGEST)
          CRITICAL=$(echo $SCAN_RESULTS | jq '.critical')

          if [ "$CRITICAL" -gt 0 ]; then
            echo "❌ Image has $CRITICAL critical CVEs, blocking deployment"
            exit 1
          fi
```

---

## Configuration Reference

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SMTP_HOST` | localhost | SMTP server hostname |
| `SMTP_PORT` | 587 | SMTP server port |
| `SMTP_USERNAME` | - | SMTP authentication username |
| `SMTP_PASSWORD` | - | SMTP authentication password |
| `SMTP_FROM` | registry@afterdarksys.com | From address for emails |

### Notification Defaults

```json
{
  "email_enabled": true,
  "cve_threshold": "HIGH",
  "immediate_notification": true,
  "only_my_images": true
}
```

---

**Questions?** Check the [GitHub Discussions](https://github.com/ryan/ads-registry/discussions) or file an [issue](https://github.com/ryan/ads-registry/issues).
