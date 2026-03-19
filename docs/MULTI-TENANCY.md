# Multi-Tenancy SaaS Architecture Guide

This guide documents the complete multi-tenancy implementation for ADS Registry, transforming it into a SaaS platform capable of hosting private registries for multiple customers.

## Architecture Overview

### Key Features

- **Schema-per-tenant isolation**: Each tenant gets their own PostgreSQL schema for complete data isolation
- **Dynamic subdomain routing**: `customer-a.registry.example.com` automatically routes to the correct tenant
- **OIDC/SSO integration**: Support for Generic OIDC and Google Workspace
- **Usage metering**: Track storage size and bandwidth for billing
- **Tenant self-service**: API endpoints for tenant administrators
- **Platform administration**: Comprehensive tenant management for platform admins

### Components

1. **Database Layer** (`migrations/015_multi_tenancy.sql`)
   - Shared `public` schema for tenant metadata
   - Separate schema per tenant (`tenant_customer_a`, `tenant_customer_b`, etc.)
   - Each tenant schema contains full registry tables (repositories, manifests, blobs, users, etc.)

2. **Tenant Service** (`internal/tenancy/tenant.go`)
   - CRUD operations for tenants
   - Tenant lookup by slug or custom domain
   - Status management (active, suspended, trial, deleted)

3. **Schema Provisioning** (`internal/tenancy/provision.go`)
   - Automatic schema creation when tenant is created
   - Copies all tables from migrations 013 & 014 into tenant schema
   - Transactional provisioning (all-or-nothing)

4. **Subdomain Middleware** (`internal/tenancy/middleware.go`)
   - Extracts tenant from HTTP Host header
   - Loads tenant context into request
   - Validates tenant status (rejects suspended/deleted tenants)

5. **Tenant-Scoped Database** (`internal/tenancy/middleware.go`)
   - Wraps `*sql.DB` with automatic schema switching
   - Sets PostgreSQL `search_path` to tenant schema
   - Provides tenant-aware transactions

6. **OIDC Service** (`internal/tenancy/oidc.go`)
   - Per-tenant OIDC provider configuration
   - OIDC discovery (auto-detects endpoints)
   - OAuth2 authorization code flow
   - ID token validation
   - Google Workspace domain restrictions

7. **Usage Metering** (`internal/tenancy/metering.go`)
   - Bandwidth tracking middleware
   - Storage calculation from blob tables
   - Aggregated metrics for billing periods
   - Real-time usage queries

8. **Management API** (`internal/api/tenancy/router.go`)
   - Platform admin endpoints (`/api/v1/platform/tenants`)
   - Tenant self-service endpoints (`/api/v1/tenant/*`)
   - OIDC configuration management
   - Usage metrics API

9. **OIDC Authentication** (`internal/api/tenancy/oidc_auth.go`)
   - OIDC login flow (`/auth/oidc/login`)
   - Callback handler (`/auth/oidc/callback`)
   - User auto-provisioning from OIDC claims

---

## Database Schema

### Shared Tables (public schema)

#### `tenants`
Core tenant information and metadata.

```sql
CREATE TABLE tenants (
    id SERIAL PRIMARY KEY,
    slug VARCHAR(63) UNIQUE NOT NULL,        -- e.g., 'customer-a'
    name VARCHAR(255) NOT NULL,              -- e.g., 'Customer A Inc.'
    schema_name VARCHAR(63) UNIQUE NOT NULL, -- e.g., 'tenant_customer_a'
    status VARCHAR(50) DEFAULT 'active',     -- active, suspended, trial, deleted, provisioning
    trial_ends_at TIMESTAMP,
    contact_email VARCHAR(255) NOT NULL,
    custom_domain VARCHAR(253),              -- Optional: registry.customer-a.com
    custom_domain_verified BOOLEAN DEFAULT FALSE,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

#### `tenant_oidc_configs`
OIDC provider configurations per tenant.

```sql
CREATE TABLE tenant_oidc_configs (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER REFERENCES tenants(id),
    provider_name VARCHAR(100) NOT NULL,     -- 'google', 'okta', 'auth0'
    provider_type VARCHAR(50) NOT NULL,      -- 'google_workspace', 'generic_oidc'
    issuer_url VARCHAR(512) NOT NULL,
    client_id VARCHAR(512) NOT NULL,
    client_secret VARCHAR(512) NOT NULL,
    scopes TEXT DEFAULT 'openid profile email',
    user_claim VARCHAR(100) DEFAULT 'email',
    allowed_domains TEXT,                    -- For Google Workspace
    enabled BOOLEAN DEFAULT TRUE,
    is_primary BOOLEAN DEFAULT FALSE
);
```

#### `tenant_usage_metrics`
Aggregated usage data for billing.

```sql
CREATE TABLE tenant_usage_metrics (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER REFERENCES tenants(id),
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,
    storage_bytes BIGINT DEFAULT 0,
    storage_bytes_max BIGINT DEFAULT 0,
    bandwidth_ingress_bytes BIGINT DEFAULT 0,
    bandwidth_egress_bytes BIGINT DEFAULT 0,
    api_requests_total BIGINT DEFAULT 0,
    active_users_count INTEGER DEFAULT 0
);
```

#### `tenant_bandwidth_events`
Detailed bandwidth event tracking.

```sql
CREATE TABLE tenant_bandwidth_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id INTEGER REFERENCES tenants(id),
    direction VARCHAR(10) NOT NULL,          -- 'ingress' or 'egress'
    bytes BIGINT NOT NULL,
    repository_path VARCHAR(512),
    resource_type VARCHAR(50),               -- 'manifest', 'blob'
    digest VARCHAR(255),
    user_id INTEGER,
    ip_address INET,
    recorded_at TIMESTAMP DEFAULT NOW()
);
```

### Tenant Schemas

Each tenant gets a complete copy of all registry tables:
- `namespaces`, `repositories`, `manifests`, `tags`, `blobs`
- `users`, `groups`, `group_members`, `repository_permissions`
- `security_scanners`, `security_scan_jobs`, `vulnerability_findings`, etc.
- All tables from migrations 013 and 014

---

## Integration Guide

### 1. Database Migration

Run the multi-tenancy migration:

```bash
psql -h localhost -U registry_user -d ads_registry -f migrations/015_multi_tenancy.sql
```

This creates:
- Shared tenant metadata tables
- The `provision_tenant_schema()` function
- A default tenant using the `public` schema (for backward compatibility)

### 2. Server Configuration

Update `cmd/ads-registry/cmd/serve.go` to wire in multi-tenancy:

```go
package cmd

import (
    "github.com/afterdarktech/ads-registry/internal/tenancy"
    tenancyAPI "github.com/afterdarktech/ads-registry/internal/api/tenancy"
    // ... other imports
)

func runServer(cfg *config.Config) error {
    // ... existing database setup ...

    // Initialize multi-tenancy services
    tenantMiddleware := tenancy.NewTenantMiddleware(
        database,
        "registry.example.com", // Base domain
        false,                   // Don't require tenant (allow default)
    )

    meteringService := tenancy.NewMeteringService(database)
    meteringMiddleware := tenancy.NewBandwidthMeteringMiddleware(meteringService)

    // Create routers
    router := mux.NewRouter()

    // Apply tenant resolution middleware FIRST
    router.Use(tenantMiddleware.Middleware)

    // Apply metering middleware SECOND
    router.Use(meteringMiddleware.Middleware)

    // Register tenant management API
    tenancyRouter := tenancyAPI.NewRouter(database)
    tenancyRouter.RegisterRoutes(router)

    // Register OIDC authentication
    oidcAuthHandler := tenancyAPI.NewOIDCAuthHandler(
        database,
        store,
        tokenService,
        "https://registry.example.com",
    )
    oidcAuthHandler.RegisterRoutes(router)

    // ... register existing v2 and management routers ...

    // ... start server ...
}
```

### 3. Configuration Updates

Add multi-tenancy config to `config.json`:

```json
{
  "multitenancy": {
    "enabled": true,
    "base_domain": "registry.example.com",
    "require_tenant": false,
    "default_tenant": "default"
  },
  "metering": {
    "enabled": true,
    "flush_interval": "30s",
    "aggregate_interval": "24h"
  }
}
```

### 4. Create Your First Tenant

Use the platform admin API:

```bash
# Create tenant
curl -X POST https://registry.example.com/api/v1/platform/tenants \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "acme",
    "name": "Acme Corporation",
    "contact_email": "admin@acme.com"
  }'

# Response:
{
  "id": 2,
  "slug": "acme",
  "name": "Acme Corporation",
  "schema_name": "tenant_acme",
  "status": "active",
  "contact_email": "admin@acme.com",
  "created_at": "2026-03-15T10:00:00Z"
}
```

The tenant is now accessible at `https://acme.registry.example.com`

### 5. Configure OIDC for a Tenant

#### Option A: Google Workspace

```bash
curl -X POST https://registry.example.com/api/v1/platform/tenants/2/oidc \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "google-workspace",
    "provider_type": "google_workspace",
    "issuer_url": "https://accounts.google.com",
    "client_id": "YOUR_GOOGLE_CLIENT_ID",
    "client_secret": "YOUR_GOOGLE_CLIENT_SECRET",
    "scopes": "openid profile email",
    "user_claim": "email",
    "allowed_domains": "acme.com",
    "enabled": true,
    "is_primary": true
  }'
```

**Google Cloud Console Setup:**
1. Go to Google Cloud Console → APIs & Services → Credentials
2. Create OAuth 2.0 Client ID (Web application)
3. Add authorized redirect URI: `https://registry.example.com/auth/oidc/callback`
4. Copy Client ID and Client Secret

#### Option B: Generic OIDC (Okta, Auth0, Keycloak)

```bash
curl -X POST https://registry.example.com/api/v1/platform/tenants/2/oidc \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "okta",
    "provider_type": "generic_oidc",
    "issuer_url": "https://dev-123456.okta.com",
    "client_id": "YOUR_OKTA_CLIENT_ID",
    "client_secret": "YOUR_OKTA_CLIENT_SECRET",
    "scopes": "openid profile email",
    "user_claim": "email",
    "enabled": true,
    "is_primary": true
  }'
```

### 6. Test OIDC Login

Users can now log in via OIDC:

```bash
# Redirect user to:
https://acme.registry.example.com/auth/oidc/login

# After authentication, they receive a JWT:
{
  "token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "username": "user@acme.com",
  "expires": 1710504000
}

# Use the token for Docker registry access:
docker login acme.registry.example.com
Username: user@acme.com
Password: <paste-jwt-token>
```

### 7. Usage Metrics & Billing

#### Get Current Usage

```bash
curl https://registry.example.com/api/v1/platform/tenants/2/usage/current \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Response:
{
  "tenant_id": 2,
  "storage_bytes": 5368709120,      # 5 GB
  "bandwidth_ingress_bytes": 1073741824,  # 1 GB uploaded
  "bandwidth_egress_bytes": 3221225472,   # 3 GB downloaded
  "repositories": 42,
  "active_users": 15
}
```

#### Aggregate Monthly Metrics (Cron Job)

```go
// scripts/cron/aggregate_usage.go
package main

import (
    "context"
    "time"

    "github.com/afterdarktech/ads-registry/internal/tenancy"
)

func main() {
    // ... setup DB connection ...

    meteringService := tenancy.NewMeteringService(db)

    // Get all active tenants
    tenants, _ := tenantService.ListTenants(ctx, 1000, 0)

    // Aggregate usage for last month
    now := time.Now()
    periodStart := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
    periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

    for _, tenant := range tenants {
        err := meteringService.AggregateUsageMetrics(
            ctx,
            tenant.ID,
            tenant.SchemaName,
            periodStart,
            periodEnd,
        )
        if err != nil {
            log.Printf("Failed to aggregate metrics for tenant %s: %v", tenant.Slug, err)
        }
    }
}
```

Schedule with cron:
```cron
0 0 1 * * /usr/local/bin/aggregate_usage
```

---

## API Reference

### Platform Admin Endpoints

All require platform admin authentication.

#### List Tenants
```http
GET /api/v1/platform/tenants?limit=50&offset=0
```

#### Create Tenant
```http
POST /api/v1/platform/tenants
Content-Type: application/json

{
  "slug": "customer-b",
  "name": "Customer B Ltd",
  "contact_email": "admin@customerb.com"
}
```

#### Get Tenant
```http
GET /api/v1/platform/tenants/{id}
```

#### Update Tenant
```http
PUT /api/v1/platform/tenants/{id}
Content-Type: application/json

{
  "name": "Customer B Corporation",
  "billing_email": "billing@customerb.com"
}
```

#### Suspend Tenant
```http
POST /api/v1/platform/tenants/{id}/suspend
```

#### Activate Tenant
```http
POST /api/v1/platform/tenants/{id}/activate
```

#### Delete Tenant
```http
DELETE /api/v1/platform/tenants/{id}?drop_schema=true
```

#### List OIDC Configs
```http
GET /api/v1/platform/tenants/{id}/oidc
```

#### Create OIDC Config
```http
POST /api/v1/platform/tenants/{id}/oidc
Content-Type: application/json

{
  "provider_name": "google",
  "provider_type": "google_workspace",
  "issuer_url": "https://accounts.google.com",
  "client_id": "...",
  "client_secret": "...",
  "allowed_domains": "example.com"
}
```

### Tenant Self-Service Endpoints

Require tenant admin authentication (tenant context from subdomain).

#### Get Current Tenant Info
```http
GET /api/v1/tenant/info
```

#### Update Current Tenant
```http
PUT /api/v1/tenant/info
Content-Type: application/json

{
  "contact_name": "John Doe",
  "billing_email": "billing@acme.com"
}
```

#### Manage OIDC Providers
```http
GET /api/v1/tenant/oidc
POST /api/v1/tenant/oidc
PUT /api/v1/tenant/oidc/{oidc_id}
DELETE /api/v1/tenant/oidc/{oidc_id}
```

#### Get Usage Metrics
```http
GET /api/v1/tenant/usage
```

---

## Tenant Lifecycle

### Provisioning Flow

1. **Create Request**
   - Platform admin calls `POST /api/v1/platform/tenants`
   - Validates slug format (DNS-safe, 1-63 chars)
   - Generates schema name: `tenant_{slug}`

2. **Database Transaction**
   - Insert tenant record with `status='provisioning'`
   - Execute `provision_tenant_schema()` function
   - Create PostgreSQL schema
   - Copy all tables from migrations 013 & 014
   - Insert default security scanners
   - Update tenant `status='active'`
   - Commit transaction (atomic)

3. **DNS Configuration**
   - Create DNS wildcard: `*.registry.example.com → YOUR_IP`
   - Or specific subdomain: `acme.registry.example.com → YOUR_IP`

4. **OIDC Setup** (optional)
   - Configure OIDC provider via API
   - Test login flow

5. **User Onboarding**
   - Send welcome email to `contact_email`
   - Provide login URL: `https://{slug}.registry.example.com/auth/oidc/login`

### Suspension Flow

```bash
POST /api/v1/platform/tenants/{id}/suspend
```

- Sets `status='suspended'`
- Middleware rejects all requests with `403 Forbidden`
- Data remains in database
- Can be reactivated with `/activate`

### Deletion Flow

```bash
DELETE /api/v1/platform/tenants/{id}?drop_schema=true
```

- Sets `status='deleted'` (soft delete)
- If `drop_schema=true`: Executes `DROP SCHEMA {schema_name} CASCADE`
- If `drop_schema=false`: Schema remains for recovery
- Tenant record remains in `tenants` table for audit trail

---

## Security Considerations

### 1. Tenant Isolation

- **Schema-level isolation**: PostgreSQL schemas provide strong isolation
- **Connection security**: Each query sets `search_path` dynamically
- **Cross-tenant leaks**: Prevented by middleware validation

### 2. OIDC Security

- **State validation**: CSRF protection via random state tokens
- **Nonce validation**: Replay attack prevention
- **ID token verification**: (Basic implementation - enhance with `coreos/go-oidc` in production)
- **Domain restrictions**: Google Workspace can restrict to specific domains

### 3. Access Control

- **Platform admins**: Separate from tenant users
- **Tenant admins**: Can manage their own OIDC and users
- **Tenant users**: Scoped to their tenant schema only

### 4. Secrets Management

- **Client secrets**: Should be encrypted at rest (use Vault integration)
- **Session keys**: Use strong random keys, rotate regularly
- **JWT signing keys**: Protect private keys, consider HSM for production

---

## Performance Considerations

### Database Connection Pooling

Each tenant query temporarily switches schema using `SET search_path`. This is efficient but consider:

```go
// Current: Dynamic schema switching per query
db.Exec("SET search_path TO tenant_acme, public")
db.Query("SELECT * FROM repositories")

// Alternative: Connection pool per tenant (for high-traffic tenants)
type TenantConnectionPool struct {
    pools map[int]*sql.DB
}
```

### Caching

Add Redis caching for:
- Tenant lookups (slug → tenant object)
- OIDC configurations
- Usage metrics (5-minute TTL)

```go
// Example with Redis
func (s *TenantService) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
    // Check cache
    cacheKey := fmt.Sprintf("tenant:slug:%s", slug)
    if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil {
        var tenant Tenant
        json.Unmarshal([]byte(cached), &tenant)
        return &tenant, nil
    }

    // Cache miss - fetch from DB
    tenant, err := s.fetchTenantBySlugFromDB(ctx, slug)
    if err != nil {
        return nil, err
    }

    // Store in cache (5 min TTL)
    cached, _ := json.Marshal(tenant)
    s.redis.Set(ctx, cacheKey, cached, 5*time.Minute)

    return tenant, nil
}
```

### Bandwidth Metering

Current implementation buffers events in memory and flushes every 30 seconds. For very high traffic:

- Use a message queue (Redis Streams, Kafka)
- Batch insert bandwidth events
- Partition `tenant_bandwidth_events` table by month

---

## Monitoring & Observability

### Metrics to Track

1. **Tenant Metrics**
   - Active tenants count
   - Trial tenants count
   - Suspended/deleted tenants

2. **Usage Metrics**
   - Total storage across all tenants
   - Total bandwidth per day
   - API request rate per tenant

3. **Performance Metrics**
   - Tenant lookup latency
   - Schema switch overhead
   - OIDC authentication latency

### Prometheus Integration

```go
// Add to main server setup
var (
    tenantCount = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ads_registry_tenants_total",
            Help: "Number of tenants by status",
        },
        []string{"status"},
    )

    tenantStorageBytes = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ads_registry_tenant_storage_bytes",
            Help: "Storage used per tenant",
        },
        []string{"tenant_slug"},
    )
)

// Update periodically
func updateTenantMetrics() {
    tenants, _ := tenantService.ListTenants(ctx, 10000, 0)
    statusCounts := make(map[string]int)

    for _, t := range tenants {
        statusCounts[t.Status]++

        usage, _ := meteringService.GetCurrentUsage(ctx, t.ID, t.SchemaName)
        tenantStorageBytes.WithLabelValues(t.Slug).Set(float64(usage.StorageBytes))
    }

    for status, count := range statusCounts {
        tenantCount.WithLabelValues(status).Set(float64(count))
    }
}
```

---

## Roadmap & Future Enhancements

### Phase 1 (Implemented)
- ✅ Schema-per-tenant isolation
- ✅ Subdomain routing
- ✅ OIDC (Generic + Google Workspace)
- ✅ Usage metering (storage + bandwidth)
- ✅ Tenant management API

### Phase 2 (Recommended Next Steps)
- [ ] Tenant admin UI (React dashboard)
- [ ] Custom domain support with automatic SSL (Let's Encrypt)
- [ ] Stripe integration for billing
- [ ] Enhanced OIDC with `coreos/go-oidc` library
- [ ] Azure AD and AWS Cognito providers
- [ ] Tenant usage dashboards and charts

### Phase 3 (Advanced)
- [ ] Multi-region tenant deployment
- [ ] Tenant migration tools
- [ ] Per-tenant feature flags
- [ ] Tenant-specific rate limiting
- [ ] Compliance exports (SOC2, GDPR)

---

## Troubleshooting

### Common Issues

#### 1. Tenant Not Found
**Symptom**: `404 Tenant not found`

**Check**:
```bash
# Verify DNS resolves
nslookup acme.registry.example.com

# Check tenant exists
psql -c "SELECT * FROM tenants WHERE slug = 'acme';"

# Check host header
curl -v https://acme.registry.example.com/v2/
```

#### 2. OIDC Login Fails
**Symptom**: `Invalid state parameter`

**Check**:
- Session cookies are enabled
- Redirect URI matches exactly
- State token stored in session

**Debug**:
```go
log.Printf("Stored state: %s", storedState)
log.Printf("Received state: %s", receivedState)
```

#### 3. Schema Not Provisioned
**Symptom**: `relation "repositories" does not exist`

**Fix**:
```sql
-- Check if schema exists
SELECT schema_name FROM information_schema.schemata WHERE schema_name = 'tenant_acme';

-- Manually provision
SELECT provision_tenant_schema('tenant_acme');

-- Verify tables
\dt tenant_acme.*
```

#### 4. Usage Metrics Not Updating
**Check**:
```bash
# Verify metering middleware is active
curl -v https://acme.registry.example.com/v2/

# Check bandwidth events table
psql -c "SELECT COUNT(*) FROM tenant_bandwidth_events WHERE tenant_id = 2;"

# Manually aggregate
./scripts/cron/aggregate_usage
```

---

## Support & Contributing

For questions, issues, or contributions related to multi-tenancy:

- **Documentation**: This file
- **Database Schema**: `migrations/015_multi_tenancy.sql`
- **Source Code**: `internal/tenancy/`, `internal/api/tenancy/`
- **Issues**: https://github.com/afterdarktech/ads-registry/issues

---

**Implementation Status**: ✅ Core multi-tenancy complete
**Last Updated**: 2026-03-15
**Version**: 1.0.0
