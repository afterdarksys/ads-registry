# TLS Nuclear Options - Certificate Path Validation

## ⚠️ Security Warning

These are "ugly but necessary" TLS compatibility options inspired by OpenSSL's `SSL_CTX_set_verify_depth`. They reduce security in exchange for compatibility with broken TLS implementations.

**DO NOT ENABLE THESE OPTIONS UNLESS:**
- You have clients with broken/incomplete certificate chains
- You cannot fix the client implementation
- You understand the security implications
- You have documented the business justification

## What This Solves

### Problem: Incomplete Certificate Chains

Some clients fail to send complete certificate chains:

```
❌ Broken Client Chain:
Client Cert → (missing intermediate) → Root CA
           ↑
      Registry sees only this, validation fails

✓ Complete Chain:
Client Cert → Intermediate CA → Sub CA → Root CA
```

### Problem: Cross-Signed Certificates

Corporate environments with complex CA hierarchies:

```
Client Cert → Corporate Intermediate
            ↓
    (cross-signed by two different roots)
            ↓
     Validation loop or confusion
```

### Problem: MITM Proxies

Corporate proxies that intercept TLS:

```
Original:  Client → Registry
With MITM: Client → Proxy (custom CA) → Registry
                      ↑
                Proxy uses its own certificate chain
```

## Configuration Options

### 1. Enable Certificate Path Validation

```json
{
  "compatibility": {
    "tls_compatibility": {
      "enable_certificate_path_validation": true
    }
  }
}
```

**What it does:**
- Activates the certificate depth limiting system
- Allows control over how far up the chain to validate

**Security impact:**
- When disabled (default): Full chain validation to root (most secure)
- When enabled: Allows limiting validation depth (less secure, more compatible)

### 2. Set Validation Depth

```json
{
  "compatibility": {
    "tls_compatibility": {
      "enable_certificate_path_validation": true,
      "certificate_path_validation_depth": 2
    }
  }
}
```

**Depth Values:**

| Depth | What Gets Validated | Security Level | Use Case |
|-------|---------------------|----------------|----------|
| `-1`  | Full chain to root  | ✅ Most Secure | Default, production |
| `0`   | Only leaf cert      | ⛔ INSECURE - DO NOT USE | Never use this |
| `1`   | Leaf + 1 intermediate | ⚠️ Low Security | Dev environments only |
| `2`   | Leaf + 2 intermediates | ⚠️ Reduced Security | Broken clients |
| `3`   | Leaf + 3 intermediates | ⚠️ Reduced Security | Complex hierarchies |

**Examples:**

```
depth=1: Validates leaf cert and ONE intermediate
  Client Cert ✓ → Intermediate CA ✓ → (stop here, don't check root)

depth=2: Validates leaf cert and TWO intermediates
  Client Cert ✓ → Intermediate CA ✓ → Sub-CA ✓ → (stop here)

depth=-1 (default): Validates entire chain
  Client Cert ✓ → Intermediate CA ✓ → Sub-CA ✓ → Root CA ✓
```

### 3. Allow Incomplete Chains

```json
{
  "compatibility": {
    "tls_compatibility": {
      "allow_incomplete_chains": true
    }
  }
}
```

**What it does:**
- Allows clients to omit intermediate certificates
- Registry attempts to build chain from known intermediates
- Uses registry's intermediate certificate cache

**Security impact:**
- ⚠️ Trusts the registry's intermediate cache
- ⚠️ Susceptible to cache poisoning attacks
- ⚠️ Client can omit validation steps

**When to use:**
- Clients that don't send intermediates (old Docker versions)
- Bandwidth-constrained environments
- Private networks with trusted client base

### 4. Accept Self-Signed Certificates

```json
{
  "compatibility": {
    "tls_compatibility": {
      "enable_certificate_path_validation": true,
      "accept_self_signed_from_clients": [
        "docker-dev-.*",
        "localhost:.*",
        "192\\.168\\..*"
      ]
    }
  }
}
```

**What it does:**
- Allows specific clients (by regex) to use self-signed certificates
- Bypasses full chain validation for matched clients

**Security impact:**
- ⛔ **CRITICAL SECURITY RISK**
- Anyone matching the pattern can connect with self-signed cert
- No protection against MITM attacks
- **NEVER use in production**

**Valid use cases:**
- Local development environments
- CI/CD test suites
- Isolated lab networks

## Real-World Scenarios

### Scenario 1: Docker on Corporate Network

**Problem:**
```
Docker client connects through corporate MITM proxy
Proxy uses custom CA with 4-level hierarchy
Docker sends incomplete chain, validation fails
```

**Solution:**
```json
{
  "compatibility": {
    "tls_compatibility": {
      "enable_certificate_path_validation": true,
      "certificate_path_validation_depth": 2,
      "allow_incomplete_chains": true
    }
  }
}
```

**Justification:**
- Depth=2 handles most corporate hierarchies
- Incomplete chains allowed because proxy doesn't send full chain
- Document: "Workaround for Acme Corp MITM proxy (ticket #12345)"

### Scenario 2: Old Containerd Version

**Problem:**
```
Containerd 1.4.x doesn't properly send intermediate certificates
Clients fail authentication despite having valid certs
Cannot upgrade containerd (frozen production environment)
```

**Solution:**
```json
{
  "compatibility": {
    "tls_compatibility": {
      "allow_incomplete_chains": true
    }
  }
}
```

**Justification:**
- Only enables incomplete chain handling
- Doesn't reduce validation depth
- Plan to remove when containerd upgraded

### Scenario 3: Development Environment

**Problem:**
```
Developers use self-signed certificates for local testing
Setting up CA infrastructure is overkill for dev
Need quick iteration without certificate hassle
```

**Solution:**
```json
{
  "compatibility": {
    "tls_compatibility": {
      "enable_certificate_path_validation": true,
      "accept_self_signed_from_clients": [
        "localhost:.*",
        "127\\.0\\.0\\..*",
        "::1"
      ],
      "certificate_path_validation_depth": 1
    }
  }
}
```

**Justification:**
- ONLY in dev environment config
- Restricted to localhost patterns
- **MUST NOT be used in production**

## Implementation Details

### How It Works

The registry uses Go's `crypto/tls` with custom verification:

```go
tlsConfig := &tls.Config{
    VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
        depth := compatConfig.GetValidationDepth()

        if depth == -1 {
            // Standard behavior: validate full chain
            return standardValidation(rawCerts, verifiedChains)
        }

        // Limited depth validation
        for _, chain := range verifiedChains {
            if len(chain) <= depth+1 {
                return nil // Chain is within acceptable depth
            }
        }

        return errors.New("certificate chain exceeds validation depth")
    },
}
```

### Metrics

The compatibility system tracks TLS workaround usage:

```prometheus
# Certificate validation depth overrides
ads_registry_compat_tls_depth_override_total{depth="2"} 42

# Self-signed certificate acceptances
ads_registry_compat_tls_self_signed_total{client_pattern="localhost:.*"} 15

# Incomplete chain allowances
ads_registry_compat_tls_incomplete_chain_total 8
```

### Logging

```
[COMPAT] [TLS] Client docker-dev-1.local matched self-signed exception pattern: "docker-dev-.*"
[COMPAT] [TLS] Limiting certificate validation depth to 2 for client 192.168.1.100
[COMPAT] [TLS] Accepting incomplete certificate chain from containerd/1.4.3
```

## Security Best Practices

### ✅ DO

1. **Document each exception:**
   ```json
   {
     "_comment": "Depth=2 required for Acme Corp MITM proxy (ticket #12345)",
     "certificate_path_validation_depth": 2
   }
   ```

2. **Use most restrictive setting possible:**
   - Start with depth=-1 (unlimited)
   - Only reduce if absolutely necessary
   - Test to find minimum required depth

3. **Limit by client pattern:**
   ```json
   {
     "accept_self_signed_from_clients": [
       "specific-broken-client:.*"  // Not ".*" (everything)
     ]
   }
   ```

4. **Monitor metrics:**
   - Alert on unexpected self-signed acceptances
   - Track depth override frequency
   - Review logs for abuse patterns

5. **Plan removal:**
   - Document when workaround can be removed
   - Schedule client upgrades
   - Set calendar reminders to review

### ❌ DON'T

1. **Don't use in production without approval:**
   - Requires security team sign-off
   - Document business justification
   - Get management approval for security exception

2. **Don't use broad patterns:**
   ```json
   {
     "accept_self_signed_from_clients": [
       ".*"  // ❌ NEVER DO THIS - accepts anything
     ]
   }
   ```

3. **Don't set depth=0:**
   ```json
   {
     "certificate_path_validation_depth": 0  // ❌ Only validates leaf cert
   }
   ```

4. **Don't leave enabled indefinitely:**
   - Set expiration dates
   - Create follow-up tickets
   - Review quarterly

5. **Don't combine all workarounds:**
   ```json
   {
     // ❌ This is basically disabling TLS security
     "enable_certificate_path_validation": true,
     "certificate_path_validation_depth": 1,
     "allow_incomplete_chains": true,
     "accept_self_signed_from_clients": [".*"]
   }
   ```

## Testing

### Test Incomplete Chain Handling

```bash
# Create client cert without intermediates
openssl req -x509 -newkey rsa:2048 \
  -keyout client.key -out client.crt -days 365

# Try connecting (should fail with depth validation)
curl --cert client.crt --key client.key \
  https://registry.example.com/v2/

# Enable incomplete chains, retry (should succeed)
```

### Test Depth Limiting

```bash
# Create chain: leaf → intermediate1 → intermediate2 → root
# (4 levels deep)

# Test with depth=2 (should fail - chain too deep)
# Test with depth=3 (should succeed)
# Test with depth=-1 (should succeed - full validation)
```

### Monitor Workaround Usage

```bash
# Check metrics for TLS workarounds
curl http://registry:5005/metrics | grep compat_tls

# Check logs for TLS exceptions
journalctl -u ads-registry | grep "COMPAT.*TLS"
```

## Migration Path

### Phase 1: Identify Problem (Week 1)
```
1. Enable observability
2. Document client failures
3. Identify root cause (incomplete chain? depth? self-signed?)
```

### Phase 2: Temporary Fix (Week 1-2)
```
1. Enable minimal workaround
2. Test with affected clients
3. Monitor metrics for unexpected usage
4. Document in change ticket
```

### Phase 3: Permanent Solution (Month 1-3)
```
1. Fix client certificate chains
2. Update client software
3. Deploy proper CA infrastructure
4. Set deadline for workaround removal
```

### Phase 4: Removal (Month 3-6)
```
1. Verify all clients fixed
2. Announce deprecation
3. Disable workaround
4. Monitor for failures
5. Remove config option
```

## Compliance Considerations

### PCI-DSS
- Custom validation may violate requirement 4.1
- Requires compensating controls
- Document in compliance audit

### SOC 2
- Trust Services Criteria CC6.6 may be affected
- Include in risk register
- Approval required from security officer

### HIPAA
- May impact administrative safeguards (§164.308)
- Document in security risk analysis
- Include in breach notification procedures

## Support

### Troubleshooting

**Q: Client still fails with depth=2?**
```bash
# Capture TLS handshake
openssl s_client -connect registry:5006 -showcerts

# Count certificates in chain
# Adjust depth accordingly
```

**Q: How do I know what depth to use?**
```bash
# Examine client certificate chain
openssl s_client -connect client:port -showcerts | \
  grep -c "BEGIN CERTIFICATE"

# Set depth = (count - 1) for safety margin
```

**Q: Self-signed pattern not matching?**
```bash
# Enable debug logging
# Check pattern compilation errors
# Verify client identifier format
```

## References

- OpenSSL: `man SSL_CTX_set_verify_depth`
- RFC 5280: X.509 Certificate Path Validation
- Postfix: smtp_tls_verify_depth
- [NIST SP 800-52r2](https://csrc.nist.gov/publications/detail/sp/800-52/rev-2/final)
