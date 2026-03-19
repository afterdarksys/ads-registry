package tenancy

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Tenant represents a multi-tenant SaaS tenant
type Tenant struct {
	ID                   int       `json:"id"`
	Slug                 string    `json:"slug"`
	Name                 string    `json:"name"`
	SchemaName           string    `json:"schema_name"`
	Status               string    `json:"status"`
	TrialEndsAt          *time.Time `json:"trial_ends_at,omitempty"`
	ContactEmail         string    `json:"contact_email"`
	ContactName          string    `json:"contact_name,omitempty"`
	BillingEmail         string    `json:"billing_email,omitempty"`
	CustomDomain         string    `json:"custom_domain,omitempty"`
	CustomDomainVerified bool      `json:"custom_domain_verified"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	CreatedByID          *int      `json:"created_by_id,omitempty"`
}

// TenantStatus represents the lifecycle status of a tenant
type TenantStatus string

const (
	TenantStatusActive       TenantStatus = "active"
	TenantStatusSuspended    TenantStatus = "suspended"
	TenantStatusTrial        TenantStatus = "trial"
	TenantStatusDeleted      TenantStatus = "deleted"
	TenantStatusProvisioning TenantStatus = "provisioning"
)

// OIDCConfig represents OIDC provider configuration for a tenant
type OIDCConfig struct {
	ID                     int       `json:"id"`
	TenantID               int       `json:"tenant_id"`
	ProviderName           string    `json:"provider_name"`
	ProviderType           string    `json:"provider_type"`
	IssuerURL              string    `json:"issuer_url"`
	ClientID               string    `json:"client_id"`
	ClientSecret           string    `json:"-"` // Never serialize
	AuthorizationEndpoint  string    `json:"authorization_endpoint,omitempty"`
	TokenEndpoint          string    `json:"token_endpoint,omitempty"`
	UserInfoEndpoint       string    `json:"userinfo_endpoint,omitempty"`
	JWKSURI                string    `json:"jwks_uri,omitempty"`
	Scopes                 string    `json:"scopes"`
	UserClaim              string    `json:"user_claim"`
	GroupClaim             string    `json:"group_claim,omitempty"`
	AllowedDomains         string    `json:"allowed_domains,omitempty"`
	Enabled                bool      `json:"enabled"`
	IsPrimary              bool      `json:"is_primary"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	CreatedByID            *int      `json:"created_by_id,omitempty"`
}

// ProviderType constants
const (
	ProviderTypeGoogleWorkspace = "google_workspace"
	ProviderTypeGenericOIDC     = "generic_oidc"
	ProviderTypeAzureAD         = "azure_ad"
	ProviderTypeAWSCognito      = "aws_cognito"
)

// UsageMetrics represents aggregated usage data for a tenant
type UsageMetrics struct {
	ID                   int       `json:"id"`
	TenantID             int       `json:"tenant_id"`
	PeriodStart          time.Time `json:"period_start"`
	PeriodEnd            time.Time `json:"period_end"`
	StorageBytes         int64     `json:"storage_bytes"`
	StorageBytesMax      int64     `json:"storage_bytes_max"`
	BlobCount            int       `json:"blob_count"`
	ManifestCount        int       `json:"manifest_count"`
	RepositoryCount      int       `json:"repository_count"`
	BandwidthIngressBytes int64    `json:"bandwidth_ingress_bytes"`
	BandwidthEgressBytes  int64    `json:"bandwidth_egress_bytes"`
	APIRequestsTotal     int64     `json:"api_requests_total"`
	APIRequestsPull      int64     `json:"api_requests_pull"`
	APIRequestsPush      int64     `json:"api_requests_push"`
	ActiveUsersCount     int       `json:"active_users_count"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// TenantService handles tenant management operations
type TenantService struct {
	db *sql.DB
}

// NewTenantService creates a new tenant service
func NewTenantService(db *sql.DB) *TenantService {
	return &TenantService{db: db}
}

// GetTenantBySlug retrieves a tenant by its subdomain slug
func (s *TenantService) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	query := `
		SELECT id, slug, name, schema_name, status, trial_ends_at,
		       contact_email, contact_name, billing_email,
		       custom_domain, custom_domain_verified,
		       created_at, updated_at, created_by_id
		FROM tenants
		WHERE slug = $1 AND status != 'deleted'
	`

	tenant := &Tenant{}
	err := s.db.QueryRowContext(ctx, query, slug).Scan(
		&tenant.ID,
		&tenant.Slug,
		&tenant.Name,
		&tenant.SchemaName,
		&tenant.Status,
		&tenant.TrialEndsAt,
		&tenant.ContactEmail,
		&tenant.ContactName,
		&tenant.BillingEmail,
		&tenant.CustomDomain,
		&tenant.CustomDomainVerified,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.CreatedByID,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant not found: %s", slug)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return tenant, nil
}

// GetTenantByCustomDomain retrieves a tenant by its custom domain
func (s *TenantService) GetTenantByCustomDomain(ctx context.Context, domain string) (*Tenant, error) {
	query := `
		SELECT id, slug, name, schema_name, status, trial_ends_at,
		       contact_email, contact_name, billing_email,
		       custom_domain, custom_domain_verified,
		       created_at, updated_at, created_by_id
		FROM tenants
		WHERE custom_domain = $1 AND custom_domain_verified = true AND status != 'deleted'
	`

	tenant := &Tenant{}
	err := s.db.QueryRowContext(ctx, query, domain).Scan(
		&tenant.ID,
		&tenant.Slug,
		&tenant.Name,
		&tenant.SchemaName,
		&tenant.Status,
		&tenant.TrialEndsAt,
		&tenant.ContactEmail,
		&tenant.ContactName,
		&tenant.BillingEmail,
		&tenant.CustomDomain,
		&tenant.CustomDomainVerified,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.CreatedByID,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant not found for domain: %s", domain)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant by domain: %w", err)
	}

	return tenant, nil
}

// CreateTenant creates a new tenant and provisions its database schema
func (s *TenantService) CreateTenant(ctx context.Context, slug, name, contactEmail string, createdByID *int) (*Tenant, error) {
	// Validate slug format (DNS-safe)
	if !isValidSlug(slug) {
		return nil, fmt.Errorf("invalid slug format: must be lowercase alphanumeric with hyphens, 1-63 characters")
	}

	// Generate schema name
	schemaName := fmt.Sprintf("tenant_%s", slug)

	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert tenant record with provisioning status
	query := `
		INSERT INTO tenants (slug, name, schema_name, status, contact_email, created_by_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	tenant := &Tenant{
		Slug:         slug,
		Name:         name,
		SchemaName:   schemaName,
		Status:       string(TenantStatusProvisioning),
		ContactEmail: contactEmail,
		CreatedByID:  createdByID,
	}

	err = tx.QueryRowContext(ctx, query,
		tenant.Slug,
		tenant.Name,
		tenant.SchemaName,
		tenant.Status,
		tenant.ContactEmail,
		tenant.CreatedByID,
	).Scan(&tenant.ID, &tenant.CreatedAt, &tenant.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create tenant record: %w", err)
	}

	// Provision the tenant schema
	if err := provisionTenantSchema(ctx, tx, schemaName); err != nil {
		return nil, fmt.Errorf("failed to provision tenant schema: %w", err)
	}

	// Update tenant status to active
	_, err = tx.ExecContext(ctx, "UPDATE tenants SET status = $1 WHERE id = $2", TenantStatusActive, tenant.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to activate tenant: %w", err)
	}
	tenant.Status = string(TenantStatusActive)

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return tenant, nil
}

// ListTenants returns all tenants (paginated)
func (s *TenantService) ListTenants(ctx context.Context, limit, offset int) ([]*Tenant, error) {
	query := `
		SELECT id, slug, name, schema_name, status, trial_ends_at,
		       contact_email, contact_name, billing_email,
		       custom_domain, custom_domain_verified,
		       created_at, updated_at, created_by_id
		FROM tenants
		WHERE status != 'deleted'
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*Tenant
	for rows.Next() {
		tenant := &Tenant{}
		err := rows.Scan(
			&tenant.ID,
			&tenant.Slug,
			&tenant.Name,
			&tenant.SchemaName,
			&tenant.Status,
			&tenant.TrialEndsAt,
			&tenant.ContactEmail,
			&tenant.ContactName,
			&tenant.BillingEmail,
			&tenant.CustomDomain,
			&tenant.CustomDomainVerified,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
			&tenant.CreatedByID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, tenant)
	}

	return tenants, nil
}

// DeleteTenant marks a tenant as deleted and optionally drops its schema
func (s *TenantService) DeleteTenant(ctx context.Context, tenantID int, dropSchema bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Get tenant schema name
	var schemaName string
	err = tx.QueryRowContext(ctx, "SELECT schema_name FROM tenants WHERE id = $1", tenantID).Scan(&schemaName)
	if err != nil {
		return fmt.Errorf("failed to get tenant schema: %w", err)
	}

	// Mark tenant as deleted
	_, err = tx.ExecContext(ctx, "UPDATE tenants SET status = $1, updated_at = NOW() WHERE id = $2", TenantStatusDeleted, tenantID)
	if err != nil {
		return fmt.Errorf("failed to mark tenant as deleted: %w", err)
	}

	// Optionally drop the schema
	if dropSchema && schemaName != "public" {
		_, err = tx.ExecContext(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
		if err != nil {
			return fmt.Errorf("failed to drop tenant schema: %w", err)
		}
	}

	return tx.Commit()
}

// isValidSlug validates tenant slug format (DNS-safe subdomain)
func isValidSlug(slug string) bool {
	// DNS subdomain: lowercase alphanumeric with hyphens, 1-63 chars
	// Cannot start or end with hyphen
	match, _ := regexp.MatchString(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`, slug)
	return match
}

// GetTenantByID retrieves a tenant by ID
func (s *TenantService) GetTenantByID(ctx context.Context, tenantID int) (*Tenant, error) {
	query := `
		SELECT id, slug, name, schema_name, status, trial_ends_at,
		       contact_email, contact_name, billing_email,
		       custom_domain, custom_domain_verified,
		       created_at, updated_at, created_by_id
		FROM tenants
		WHERE id = $1
	`

	tenant := &Tenant{}
	err := s.db.QueryRowContext(ctx, query, tenantID).Scan(
		&tenant.ID,
		&tenant.Slug,
		&tenant.Name,
		&tenant.SchemaName,
		&tenant.Status,
		&tenant.TrialEndsAt,
		&tenant.ContactEmail,
		&tenant.ContactName,
		&tenant.BillingEmail,
		&tenant.CustomDomain,
		&tenant.CustomDomainVerified,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.CreatedByID,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant not found: %d", tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return tenant, nil
}

// UpdateTenant updates tenant information
func (s *TenantService) UpdateTenant(ctx context.Context, tenantID int, updates interface{}) (*Tenant, error) {
	// Simple implementation - can be enhanced with dynamic query building
	query := `
		UPDATE tenants
		SET name = COALESCE(NULLIF($1, ''), name),
		    contact_email = COALESCE(NULLIF($2, ''), contact_email),
		    contact_name = COALESCE(NULLIF($3, ''), contact_name),
		    billing_email = COALESCE(NULLIF($4, ''), billing_email),
		    updated_at = NOW()
		WHERE id = $5
	`

	// Type assertion for updates
	updateMap, ok := updates.(*struct {
		Name         string `json:"name,omitempty"`
		ContactEmail string `json:"contact_email,omitempty"`
		ContactName  string `json:"contact_name,omitempty"`
		BillingEmail string `json:"billing_email,omitempty"`
	})
	if !ok {
		return nil, fmt.Errorf("invalid update structure")
	}

	_, err := s.db.ExecContext(ctx, query,
		updateMap.Name,
		updateMap.ContactEmail,
		updateMap.ContactName,
		updateMap.BillingEmail,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update tenant: %w", err)
	}

	return s.GetTenantByID(ctx, tenantID)
}

// UpdateTenantStatus updates tenant status
func (s *TenantService) UpdateTenantStatus(ctx context.Context, tenantID int, status TenantStatus) error {
	query := `UPDATE tenants SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := s.db.ExecContext(ctx, query, status, tenantID)
	return err
}

// ExtractTenantSlugFromHost extracts tenant slug from hostname
// Examples:
//   - "customer-a.registry.example.com" -> "customer-a"
//   - "registry.example.com" -> "default"
//   - "registry.customer-a.com" (custom domain) -> lookup required
func ExtractTenantSlugFromHost(host string) string {
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Split hostname into parts
	parts := strings.Split(host, ".")

	// If single part or no subdomain, return default
	if len(parts) <= 2 {
		return "default"
	}

	// First part is the tenant slug
	return parts[0]
}
