package tenancy

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// TenantContextKey is the context key for storing tenant information
	TenantContextKey contextKey = "tenant"
)

// TenantMiddleware extracts tenant from subdomain and loads tenant context
type TenantMiddleware struct {
	tenantService *TenantService
	baseDomain    string // e.g., "registry.example.com"
	requireTenant bool   // If true, reject requests without valid tenant
}

// NewTenantMiddleware creates a new tenant resolution middleware
func NewTenantMiddleware(db *sql.DB, baseDomain string, requireTenant bool) *TenantMiddleware {
	return &TenantMiddleware{
		tenantService: NewTenantService(db),
		baseDomain:    baseDomain,
		requireTenant: requireTenant,
	}
}

// Middleware is the HTTP middleware function
func (m *TenantMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract tenant from request
		tenant, err := m.resolveTenant(ctx, r)
		if err != nil {
			if m.requireTenant {
				log.Printf("Tenant resolution failed: %v", err)
				http.Error(w, "Tenant not found", http.StatusNotFound)
				return
			}
			// If tenant is optional, continue without tenant context
			next.ServeHTTP(w, r)
			return
		}

		// Check tenant status
		if tenant.Status == string(TenantStatusSuspended) {
			http.Error(w, "Tenant account suspended", http.StatusForbidden)
			return
		}

		if tenant.Status == string(TenantStatusDeleted) {
			http.Error(w, "Tenant not found", http.StatusNotFound)
			return
		}

		// Add tenant to request context
		ctx = context.WithValue(ctx, TenantContextKey, tenant)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// resolveTenant extracts and loads tenant from request host
func (m *TenantMiddleware) resolveTenant(ctx context.Context, r *http.Request) (*Tenant, error) {
	host := r.Host

	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Strategy 1: Check if it's a custom domain
	if host != m.baseDomain && !strings.HasSuffix(host, "."+m.baseDomain) {
		tenant, err := m.tenantService.GetTenantByCustomDomain(ctx, host)
		if err == nil {
			return tenant, nil
		}
		// If not found as custom domain, try subdomain resolution
	}

	// Strategy 2: Extract subdomain from base domain
	slug := m.extractSlugFromHost(host)

	// Load tenant by slug
	tenant, err := m.tenantService.GetTenantBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("tenant not found for slug %s: %w", slug, err)
	}

	return tenant, nil
}

// extractSlugFromHost extracts tenant slug from hostname
func (m *TenantMiddleware) extractSlugFromHost(host string) string {
	// If host equals base domain, use default tenant
	if host == m.baseDomain {
		return "default"
	}

	// Remove base domain suffix
	if strings.HasSuffix(host, "."+m.baseDomain) {
		subdomain := strings.TrimSuffix(host, "."+m.baseDomain)
		// Handle multi-level subdomains (take first part)
		parts := strings.Split(subdomain, ".")
		return parts[0]
	}

	// Fallback: return first part of hostname
	parts := strings.Split(host, ".")
	if len(parts) > 0 {
		return parts[0]
	}

	return "default"
}

// GetTenantFromContext retrieves tenant from request context
func GetTenantFromContext(ctx context.Context) (*Tenant, bool) {
	tenant, ok := ctx.Value(TenantContextKey).(*Tenant)
	return tenant, ok
}

// MustGetTenantFromContext retrieves tenant from context or panics
func MustGetTenantFromContext(ctx context.Context) *Tenant {
	tenant, ok := GetTenantFromContext(ctx)
	if !ok {
		panic("tenant not found in context")
	}
	return tenant
}

// TenantScopedDB wraps a database connection with tenant schema context
type TenantScopedDB struct {
	db     *sql.DB
	tenant *Tenant
}

// NewTenantScopedDB creates a tenant-scoped database connection
func NewTenantScopedDB(db *sql.DB, tenant *Tenant) *TenantScopedDB {
	return &TenantScopedDB{
		db:     db,
		tenant: tenant,
	}
}

// Exec executes a query in the tenant's schema
func (tdb *TenantScopedDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return tdb.execInSchema(ctx, query, args...)
}

// Query executes a query and returns rows in the tenant's schema
func (tdb *TenantScopedDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return tdb.queryInSchema(ctx, query, args...)
}

// QueryRow executes a query that returns a single row in the tenant's schema
func (tdb *TenantScopedDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return tdb.queryRowInSchema(ctx, query, args...)
}

// execInSchema executes a statement in the tenant's schema
func (tdb *TenantScopedDB) execInSchema(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	// Start transaction to ensure schema context is maintained
	tx, err := tdb.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Set search path to tenant schema
	_, err = tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL search_path TO %s, public", tdb.tenant.SchemaName))
	if err != nil {
		return nil, fmt.Errorf("failed to set search path: %w", err)
	}

	// Execute query
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return result, nil
}

// queryInSchema executes a query in the tenant's schema
func (tdb *TenantScopedDB) queryInSchema(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	// For queries, we use a connection-level search path
	// Note: This requires connection pooling per tenant or dynamic schema switching

	// Get connection from pool
	conn, err := tdb.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	// Don't close conn here - caller must close rows which will release the connection

	// Set search path for this connection
	_, err = conn.ExecContext(ctx, fmt.Sprintf("SET search_path TO %s, public", tdb.tenant.SchemaName))
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set search path: %w", err)
	}

	// Execute query
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return rows, nil
}

// queryRowInSchema executes a query that returns a single row
func (tdb *TenantScopedDB) queryRowInSchema(ctx context.Context, query string, args ...interface{}) *sql.Row {
	// Get connection from pool
	conn, err := tdb.db.Conn(ctx)
	if err != nil {
		// Return a row that will error when scanned
		return &sql.Row{}
	}
	defer conn.Close()

	// Set search path for this connection
	_, err = conn.ExecContext(ctx, fmt.Sprintf("SET search_path TO %s, public", tdb.tenant.SchemaName))
	if err != nil {
		// Return a row that will error when scanned
		return &sql.Row{}
	}

	// Execute query
	return conn.QueryRowContext(ctx, query, args...)
}

// BeginTx starts a transaction in the tenant's schema
func (tdb *TenantScopedDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*TenantTx, error) {
	tx, err := tdb.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Set search path
	_, err = tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL search_path TO %s, public", tdb.tenant.SchemaName))
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to set search path: %w", err)
	}

	return &TenantTx{tx: tx, tenant: tdb.tenant}, nil
}

// TenantTx wraps a transaction with tenant schema context
type TenantTx struct {
	tx     *sql.Tx
	tenant *Tenant
}

// Exec executes a statement in the transaction
func (ttx *TenantTx) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return ttx.tx.ExecContext(ctx, query, args...)
}

// Query executes a query in the transaction
func (ttx *TenantTx) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return ttx.tx.QueryContext(ctx, query, args...)
}

// QueryRow executes a query that returns a single row
func (ttx *TenantTx) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return ttx.tx.QueryRowContext(ctx, query, args...)
}

// Commit commits the transaction
func (ttx *TenantTx) Commit() error {
	return ttx.tx.Commit()
}

// Rollback rolls back the transaction
func (ttx *TenantTx) Rollback() error {
	return ttx.tx.Rollback()
}

// GetDB returns the underlying *sql.Tx
func (ttx *TenantTx) GetDB() *sql.Tx {
	return ttx.tx
}
