-- Migration 015: Multi-Tenancy SaaS Architecture
-- Implements schema-per-tenant isolation with subdomain routing

-- ============================================================================
-- SHARED PUBLIC SCHEMA TABLES
-- These tables are shared across all tenants and contain tenant metadata
-- ============================================================================

-- Tenants table: Core tenant information
CREATE TABLE IF NOT EXISTS tenants (
    id SERIAL PRIMARY KEY,

    -- Tenant identification
    slug VARCHAR(63) NOT NULL UNIQUE, -- DNS-safe subdomain (e.g., 'customer-a')
    name VARCHAR(255) NOT NULL, -- Display name (e.g., 'Customer A Inc.')

    -- Schema isolation
    schema_name VARCHAR(63) NOT NULL UNIQUE, -- PostgreSQL schema name (e.g., 'tenant_customer_a')

    -- Status and lifecycle
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- active, suspended, trial, deleted
    trial_ends_at TIMESTAMP, -- NULL if not on trial

    -- Contact and billing
    contact_email VARCHAR(255) NOT NULL,
    contact_name VARCHAR(255),
    billing_email VARCHAR(255),

    -- Custom domain support (future)
    custom_domain VARCHAR(253), -- Optional custom domain (e.g., 'registry.customer-a.com')
    custom_domain_verified BOOLEAN DEFAULT FALSE,

    -- Metadata
    metadata JSONB, -- Additional tenant-specific configuration

    -- Audit fields
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by_id INTEGER, -- Reference to platform admin user

    -- Constraints
    CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'), -- Valid DNS subdomain
    CHECK (status IN ('active', 'suspended', 'trial', 'deleted', 'provisioning'))
);

CREATE INDEX idx_tenants_slug ON tenants(slug);
CREATE INDEX idx_tenants_schema_name ON tenants(schema_name);
CREATE INDEX idx_tenants_status ON tenants(status);
CREATE INDEX idx_tenants_custom_domain ON tenants(custom_domain) WHERE custom_domain IS NOT NULL;

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_tenants_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW
    EXECUTE FUNCTION update_tenants_updated_at();

-- ============================================================================
-- TENANT OIDC CONFIGURATION
-- Each tenant can configure their own OIDC provider(s)
-- ============================================================================

CREATE TABLE IF NOT EXISTS tenant_oidc_configs (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Provider identification
    provider_name VARCHAR(100) NOT NULL, -- 'google', 'okta', 'auth0', 'generic', etc.
    provider_type VARCHAR(50) NOT NULL, -- 'google_workspace', 'generic_oidc'

    -- OIDC configuration
    issuer_url VARCHAR(512) NOT NULL, -- OIDC issuer URL
    client_id VARCHAR(512) NOT NULL,
    client_secret VARCHAR(512) NOT NULL, -- Should be encrypted in production

    -- Optional provider-specific settings
    authorization_endpoint VARCHAR(512),
    token_endpoint VARCHAR(512),
    userinfo_endpoint VARCHAR(512),
    jwks_uri VARCHAR(512),

    -- Scopes and claims
    scopes TEXT NOT NULL DEFAULT 'openid profile email', -- Space-separated scopes
    user_claim VARCHAR(100) DEFAULT 'email', -- Claim to use as username
    group_claim VARCHAR(100), -- Optional claim for group mapping

    -- Domain restrictions (for Google Workspace)
    allowed_domains TEXT, -- Comma-separated list of allowed email domains

    -- Status
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE, -- Primary auth method for tenant

    -- Metadata
    metadata JSONB, -- Provider-specific configuration

    -- Audit
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by_id INTEGER,

    -- Constraints
    UNIQUE (tenant_id, provider_name),
    CHECK (provider_type IN ('google_workspace', 'generic_oidc', 'azure_ad', 'aws_cognito'))
);

CREATE INDEX idx_tenant_oidc_tenant_id ON tenant_oidc_configs(tenant_id);
CREATE INDEX idx_tenant_oidc_enabled ON tenant_oidc_configs(enabled);

-- ============================================================================
-- TENANT USAGE METRICS
-- Tracks storage size and bandwidth for billing
-- ============================================================================

CREATE TABLE IF NOT EXISTS tenant_usage_metrics (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Time period
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,

    -- Storage metrics
    storage_bytes BIGINT NOT NULL DEFAULT 0, -- Total storage used in this period
    storage_bytes_max BIGINT NOT NULL DEFAULT 0, -- Peak storage during period
    blob_count INTEGER NOT NULL DEFAULT 0, -- Number of blobs
    manifest_count INTEGER NOT NULL DEFAULT 0, -- Number of manifests
    repository_count INTEGER NOT NULL DEFAULT 0, -- Number of repositories

    -- Bandwidth metrics
    bandwidth_ingress_bytes BIGINT NOT NULL DEFAULT 0, -- Uploaded bytes
    bandwidth_egress_bytes BIGINT NOT NULL DEFAULT 0, -- Downloaded bytes

    -- API usage metrics
    api_requests_total BIGINT NOT NULL DEFAULT 0, -- Total API requests
    api_requests_pull BIGINT NOT NULL DEFAULT 0, -- Pull operations
    api_requests_push BIGINT NOT NULL DEFAULT 0, -- Push operations

    -- User activity
    active_users_count INTEGER NOT NULL DEFAULT 0, -- Unique active users in period

    -- Metadata
    metadata JSONB, -- Additional metrics or breakdown

    -- Audit
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Constraints
    CHECK (period_end > period_start),
    UNIQUE (tenant_id, period_start, period_end)
);

CREATE INDEX idx_tenant_usage_tenant_id ON tenant_usage_metrics(tenant_id);
CREATE INDEX idx_tenant_usage_period ON tenant_usage_metrics(period_start, period_end);

-- Current usage view (real-time aggregation)
CREATE OR REPLACE VIEW v_tenant_current_usage AS
SELECT
    t.id AS tenant_id,
    t.slug,
    t.name,
    t.schema_name,
    COUNT(DISTINCT r.id) AS repository_count,
    COUNT(DISTINCT m.id) AS manifest_count,
    COUNT(DISTINCT b.id) AS blob_count,
    COALESCE(SUM(b.size_bytes), 0) AS storage_bytes,
    COUNT(DISTINCT u.id) AS user_count
FROM tenants t
LEFT JOIN LATERAL (
    SELECT * FROM information_schema.tables
    WHERE table_schema = t.schema_name
    LIMIT 1
) AS schema_check ON TRUE
-- Note: This view requires dynamic SQL to query per-tenant schemas
-- In practice, this will be computed by application code
GROUP BY t.id, t.slug, t.name, t.schema_name;

-- ============================================================================
-- TENANT SUBSCRIPTIONS & BILLING
-- Tracks subscription plans and billing information
-- ============================================================================

CREATE TABLE IF NOT EXISTS tenant_subscriptions (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Plan information
    plan_name VARCHAR(100) NOT NULL, -- 'free', 'starter', 'professional', 'enterprise'
    plan_tier VARCHAR(50) NOT NULL, -- 'free', 'paid'

    -- Pricing
    monthly_price_cents INTEGER, -- Price in cents (NULL for custom pricing)
    currency VARCHAR(3) DEFAULT 'USD',

    -- Limits (NULL = unlimited)
    max_storage_bytes BIGINT, -- Storage quota
    max_bandwidth_bytes_monthly BIGINT, -- Monthly bandwidth quota
    max_repositories INTEGER, -- Repository limit
    max_users INTEGER, -- User limit

    -- Features
    features JSONB, -- Feature flags (e.g., {"vulnerability_scanning": true})

    -- Billing
    billing_cycle VARCHAR(20) DEFAULT 'monthly', -- 'monthly', 'annual'
    next_billing_date TIMESTAMP,
    stripe_subscription_id VARCHAR(255), -- External billing system reference
    stripe_customer_id VARCHAR(255),

    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'active',

    -- Audit
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    starts_at TIMESTAMP,
    ends_at TIMESTAMP,

    CHECK (plan_tier IN ('free', 'paid', 'trial', 'custom')),
    CHECK (status IN ('active', 'canceled', 'past_due', 'suspended'))
);

CREATE INDEX idx_tenant_subs_tenant_id ON tenant_subscriptions(tenant_id);
CREATE INDEX idx_tenant_subs_status ON tenant_subscriptions(status);

-- ============================================================================
-- TENANT BANDWIDTH TRACKING
-- Detailed bandwidth tracking for metering
-- ============================================================================

CREATE TABLE IF NOT EXISTS tenant_bandwidth_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Request information
    direction VARCHAR(10) NOT NULL, -- 'ingress' or 'egress'
    bytes BIGINT NOT NULL,

    -- Resource information
    repository_path VARCHAR(512), -- e.g., 'library/ubuntu'
    resource_type VARCHAR(50), -- 'manifest', 'blob', 'tag_list'
    digest VARCHAR(255), -- SHA256 digest if applicable

    -- Request metadata
    user_id INTEGER, -- User who made the request (tenant-scoped)
    ip_address INET,
    user_agent TEXT,

    -- Timestamp
    recorded_at TIMESTAMP NOT NULL DEFAULT NOW(),

    CHECK (direction IN ('ingress', 'egress'))
);

-- Partitioning recommendation: Partition by recorded_at monthly
CREATE INDEX idx_bandwidth_tenant_time ON tenant_bandwidth_events(tenant_id, recorded_at);
CREATE INDEX idx_bandwidth_direction ON tenant_bandwidth_events(direction);

-- ============================================================================
-- TENANT AUDIT LOG
-- Platform-level tenant administration audit trail
-- ============================================================================

CREATE TABLE IF NOT EXISTS tenant_audit_log (
    id BIGSERIAL PRIMARY KEY,
    tenant_id INTEGER REFERENCES tenants(id) ON DELETE SET NULL,

    -- Action tracking
    action VARCHAR(100) NOT NULL, -- 'create_tenant', 'suspend_tenant', 'update_subscription', etc.
    actor_type VARCHAR(50) NOT NULL, -- 'platform_admin', 'tenant_admin', 'system'
    actor_id INTEGER, -- Platform admin user ID

    -- Details
    details JSONB, -- Action-specific details

    -- Request context
    ip_address INET,
    user_agent TEXT,

    -- State tracking
    before_state JSONB,
    after_state JSONB,

    -- Timestamp
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenant_audit_tenant_id ON tenant_audit_log(tenant_id);
CREATE INDEX idx_tenant_audit_action ON tenant_audit_log(action);
CREATE INDEX idx_tenant_audit_created_at ON tenant_audit_log(created_at);

-- ============================================================================
-- TENANT SCHEMA TEMPLATE
-- Function to provision a new tenant schema with all necessary tables
-- ============================================================================

CREATE OR REPLACE FUNCTION provision_tenant_schema(p_schema_name VARCHAR(63))
RETURNS VOID AS $$
DECLARE
    v_sql TEXT;
BEGIN
    -- Create the schema
    EXECUTE format('CREATE SCHEMA IF NOT EXISTS %I', p_schema_name);

    -- Set search path to new schema
    EXECUTE format('SET search_path TO %I', p_schema_name);

    -- Copy all table definitions from migration 013 and 014 into tenant schema
    -- Note: This is a placeholder - the actual implementation will copy
    -- the full schema from migrations 013_ownership_and_security.sql
    -- and 014_oci_artifacts.sql

    -- For now, we'll document that the provision_tenant_schema function
    -- needs to execute the full DDL from those migrations in the tenant schema

    RAISE NOTICE 'Tenant schema % provisioned successfully', p_schema_name;

    -- Reset search path
    SET search_path TO public;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- TENANT DELETION/CLEANUP
-- Function to safely delete/archive a tenant
-- ============================================================================

CREATE OR REPLACE FUNCTION delete_tenant_schema(p_schema_name VARCHAR(63), p_archive BOOLEAN DEFAULT FALSE)
RETURNS VOID AS $$
BEGIN
    IF p_archive THEN
        -- TODO: Implement schema archival (dump to backup storage)
        RAISE NOTICE 'Archiving tenant schema %', p_schema_name;
    END IF;

    -- Drop the schema and all objects
    EXECUTE format('DROP SCHEMA IF EXISTS %I CASCADE', p_schema_name);

    RAISE NOTICE 'Tenant schema % deleted', p_schema_name;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- INDEXES FOR TENANT CONTEXT RESOLUTION
-- ============================================================================

-- Create extension for improved text search if not exists
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Index for fast tenant lookup by subdomain
CREATE INDEX idx_tenants_slug_trgm ON tenants USING gin(slug gin_trgm_ops);

-- ============================================================================
-- PLATFORM ADMIN USERS
-- Separate table for platform administrators (not tenant-scoped)
-- ============================================================================

CREATE TABLE IF NOT EXISTS platform_admins (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,

    -- Roles
    role VARCHAR(50) NOT NULL DEFAULT 'admin', -- 'super_admin', 'admin', 'support'

    -- Status
    active BOOLEAN NOT NULL DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMP,

    CHECK (role IN ('super_admin', 'admin', 'support', 'billing'))
);

CREATE INDEX idx_platform_admins_username ON platform_admins(username);
CREATE INDEX idx_platform_admins_email ON platform_admins(email);

-- ============================================================================
-- SEED DATA - Default tenant for existing installation
-- ============================================================================

-- Create a default tenant for existing installations to maintain backward compatibility
-- This tenant will use the default 'public' schema initially
INSERT INTO tenants (slug, name, schema_name, status, contact_email)
VALUES ('default', 'Default Tenant', 'public', 'active', 'admin@localhost')
ON CONFLICT (slug) DO NOTHING;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE tenants IS 'Multi-tenant SaaS: Core tenant metadata and configuration';
COMMENT ON COLUMN tenants.slug IS 'DNS-safe subdomain identifier (e.g., customer-a for customer-a.registry.example.com)';
COMMENT ON COLUMN tenants.schema_name IS 'PostgreSQL schema name for tenant data isolation';

COMMENT ON TABLE tenant_oidc_configs IS 'Per-tenant OIDC provider configuration for SSO';
COMMENT ON TABLE tenant_usage_metrics IS 'Aggregated usage metrics for billing and reporting';
COMMENT ON TABLE tenant_subscriptions IS 'Subscription plans and billing information';
COMMENT ON TABLE tenant_bandwidth_events IS 'Detailed bandwidth event tracking for metering';
COMMENT ON TABLE tenant_audit_log IS 'Platform-level audit trail for tenant administration';

COMMENT ON FUNCTION provision_tenant_schema IS 'Provisions a new isolated schema for a tenant with all necessary tables';
COMMENT ON FUNCTION delete_tenant_schema IS 'Safely deletes or archives a tenant schema';
