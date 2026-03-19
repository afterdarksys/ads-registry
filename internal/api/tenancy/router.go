package tenancy

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/ryan/ads-registry/internal/tenancy"
	"github.com/go-chi/chi/v5"
)

// Router handles tenant management API endpoints
type Router struct {
	tenantService *tenancy.TenantService
	oidcService   *tenancy.OIDCService
}

// NewRouter creates a new tenant management router
func NewRouter(db *sql.DB) *Router {
	return &Router{
		tenantService: tenancy.NewTenantService(db),
		oidcService:   tenancy.NewOIDCService(db),
	}
}

// RegisterRoutes registers all tenant management routes
func (rt *Router) RegisterRoutes(router chi.Router) {
	// Platform admin routes (require platform admin authentication)
	router.Route("/api/v1/platform", func(admin chi.Router) {
		// Tenant management
		admin.Get("/tenants", rt.listTenants)
		admin.Post("/tenants", rt.createTenant)
		admin.Get("/tenants/{id}", rt.getTenant)
		admin.Put("/tenants/{id}", rt.updateTenant)
		admin.Delete("/tenants/{id}", rt.deleteTenant)
		admin.Post("/tenants/{id}/suspend", rt.suspendTenant)
		admin.Post("/tenants/{id}/activate", rt.activateTenant)

		// OIDC configuration
		admin.Get("/tenants/{id}/oidc", rt.listOIDCConfigs)
		admin.Post("/tenants/{id}/oidc", rt.createOIDCConfig)
		admin.Put("/tenants/{id}/oidc/{oidc_id}", rt.updateOIDCConfig)
		admin.Delete("/tenants/{id}/oidc/{oidc_id}", rt.deleteOIDCConfig)

		// Usage metrics
		admin.Get("/tenants/{id}/usage", rt.getTenantUsage)
		admin.Get("/tenants/{id}/usage/current", rt.getCurrentUsage)
	})

	// Tenant self-service routes (tenant admin authentication)
	router.Route("/api/v1/tenant", func(tenant chi.Router) {
		// Tenant info (current tenant from context)
		tenant.Get("/info", rt.getCurrentTenantInfo)
		tenant.Put("/info", rt.updateCurrentTenant)

		// Tenant OIDC self-management
		tenant.Get("/oidc", rt.listCurrentTenantOIDC)
		tenant.Post("/oidc", rt.createCurrentTenantOIDC)
		tenant.Put("/oidc/{oidc_id}", rt.updateCurrentTenantOIDC)
		tenant.Delete("/oidc/{oidc_id}", rt.deleteCurrentTenantOIDC)

		// Usage metrics (self-service)
		tenant.Get("/usage", rt.getCurrentTenantUsage)
	})
}

// ============================================================================
// PLATFORM ADMIN ENDPOINTS
// ============================================================================

// listTenants returns all tenants (paginated)
func (rt *Router) listTenants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse pagination
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}

	tenants, err := rt.tenantService.ListTenants(ctx, limit, offset)
	if err != nil {
		log.Printf("Failed to list tenants: %v", err)
		http.Error(w, "Failed to list tenants", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tenants": tenants,
		"limit":   limit,
		"offset":  offset,
	})
}

// createTenant creates a new tenant
func (rt *Router) createTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Slug         string `json:"slug"`
		Name         string `json:"name"`
		ContactEmail string `json:"contact_email"`
		ContactName  string `json:"contact_name,omitempty"`
		BillingEmail string `json:"billing_email,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Slug == "" || req.Name == "" || req.ContactEmail == "" {
		http.Error(w, "Missing required fields: slug, name, contact_email", http.StatusBadRequest)
		return
	}

	// Create tenant
	tenant, err := rt.tenantService.CreateTenant(ctx, req.Slug, req.Name, req.ContactEmail, nil)
	if err != nil {
		log.Printf("Failed to create tenant: %v", err)
		http.Error(w, "Failed to create tenant: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tenant)
}

// getTenant returns a specific tenant
func (rt *Router) getTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	tenant, err := rt.tenantService.GetTenantByID(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "Tenant not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenant)
}

// updateTenant updates tenant information
func (rt *Router) updateTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Name         string `json:"name,omitempty"`
		ContactEmail string `json:"contact_email,omitempty"`
		ContactName  string `json:"contact_name,omitempty"`
		BillingEmail string `json:"billing_email,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant, err := rt.tenantService.UpdateTenant(r.Context(), tenantID, &req)
	if err != nil {
		log.Printf("Failed to update tenant: %v", err)
		http.Error(w, "Failed to update tenant", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenant)
}

// deleteTenant soft-deletes a tenant
func (rt *Router) deleteTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	// Check if schema should be dropped
	dropSchema := r.URL.Query().Get("drop_schema") == "true"

	err = rt.tenantService.DeleteTenant(r.Context(), tenantID, dropSchema)
	if err != nil {
		log.Printf("Failed to delete tenant: %v", err)
		http.Error(w, "Failed to delete tenant", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// suspendTenant suspends a tenant
func (rt *Router) suspendTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	err = rt.tenantService.UpdateTenantStatus(r.Context(), tenantID, tenancy.TenantStatusSuspended)
	if err != nil {
		log.Printf("Failed to suspend tenant: %v", err)
		http.Error(w, "Failed to suspend tenant", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "suspended"})
}

// activateTenant activates a suspended tenant
func (rt *Router) activateTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	err = rt.tenantService.UpdateTenantStatus(r.Context(), tenantID, tenancy.TenantStatusActive)
	if err != nil {
		log.Printf("Failed to activate tenant: %v", err)
		http.Error(w, "Failed to activate tenant", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "active"})
}

// ============================================================================
// OIDC CONFIGURATION ENDPOINTS
// ============================================================================

// listOIDCConfigs lists OIDC providers for a tenant
func (rt *Router) listOIDCConfigs(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	configs, err := rt.oidcService.ListOIDCConfigs(r.Context(), tenantID)
	if err != nil {
		log.Printf("Failed to list OIDC configs: %v", err)
		http.Error(w, "Failed to list OIDC configs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"oidc_providers": configs,
	})
}

// createOIDCConfig creates a new OIDC provider configuration
func (rt *Router) createOIDCConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	var config tenancy.OIDCConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config.TenantID = tenantID

	created, err := rt.oidcService.CreateOIDCConfig(r.Context(), &config)
	if err != nil {
		log.Printf("Failed to create OIDC config: %v", err)
		http.Error(w, "Failed to create OIDC config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// updateOIDCConfig updates an OIDC provider configuration
func (rt *Router) updateOIDCConfig(w http.ResponseWriter, r *http.Request) {
	oidcID, err := strconv.Atoi(chi.URLParam(r, "oidc_id"))
	if err != nil {
		http.Error(w, "Invalid OIDC config ID", http.StatusBadRequest)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config, err := rt.oidcService.UpdateOIDCConfig(r.Context(), oidcID, updates)
	if err != nil {
		log.Printf("Failed to update OIDC config: %v", err)
		http.Error(w, "Failed to update OIDC config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// deleteOIDCConfig deletes an OIDC provider configuration
func (rt *Router) deleteOIDCConfig(w http.ResponseWriter, r *http.Request) {
	oidcID, err := strconv.Atoi(chi.URLParam(r, "oidc_id"))
	if err != nil {
		http.Error(w, "Invalid OIDC config ID", http.StatusBadRequest)
		return
	}

	err = rt.oidcService.DeleteOIDCConfig(r.Context(), oidcID)
	if err != nil {
		log.Printf("Failed to delete OIDC config: %v", err)
		http.Error(w, "Failed to delete OIDC config", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// USAGE METRICS ENDPOINTS
// ============================================================================

// getTenantUsage returns historical usage metrics for a tenant
func (rt *Router) getTenantUsage(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	// TODO: Implement usage metrics retrieval
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"tenant_id": tenantID,
		"usage":     []interface{}{},
	})
}

// getCurrentUsage returns real-time usage for a tenant
func (rt *Router) getCurrentUsage(w http.ResponseWriter, r *http.Request) {
	tenantID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	// TODO: Implement current usage calculation
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"tenant_id":      tenantID,
		"storage_bytes":  0,
		"bandwidth":      0,
		"repositories":   0,
		"active_users":   0,
	})
}

// ============================================================================
// TENANT SELF-SERVICE ENDPOINTS
// ============================================================================

// getCurrentTenantInfo returns information about the current tenant
func (rt *Router) getCurrentTenantInfo(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenancy.GetTenantFromContext(r.Context())
	if !ok {
		http.Error(w, "Tenant context not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tenant)
}

// updateCurrentTenant updates the current tenant's information
func (rt *Router) updateCurrentTenant(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenancy.GetTenantFromContext(r.Context())
	if !ok {
		http.Error(w, "Tenant context not found", http.StatusInternalServerError)
		return
	}

	var req struct {
		Name         string `json:"name,omitempty"`
		ContactEmail string `json:"contact_email,omitempty"`
		ContactName  string `json:"contact_name,omitempty"`
		BillingEmail string `json:"billing_email,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	updated, err := rt.tenantService.UpdateTenant(r.Context(), tenant.ID, &req)
	if err != nil {
		log.Printf("Failed to update tenant: %v", err)
		http.Error(w, "Failed to update tenant", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// listCurrentTenantOIDC lists OIDC providers for current tenant
func (rt *Router) listCurrentTenantOIDC(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenancy.GetTenantFromContext(r.Context())
	if !ok {
		http.Error(w, "Tenant context not found", http.StatusInternalServerError)
		return
	}

	configs, err := rt.oidcService.ListOIDCConfigs(r.Context(), tenant.ID)
	if err != nil {
		log.Printf("Failed to list OIDC configs: %v", err)
		http.Error(w, "Failed to list OIDC configs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"oidc_providers": configs,
	})
}

// createCurrentTenantOIDC creates OIDC config for current tenant
func (rt *Router) createCurrentTenantOIDC(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenancy.GetTenantFromContext(r.Context())
	if !ok {
		http.Error(w, "Tenant context not found", http.StatusInternalServerError)
		return
	}

	var config tenancy.OIDCConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config.TenantID = tenant.ID

	created, err := rt.oidcService.CreateOIDCConfig(r.Context(), &config)
	if err != nil {
		log.Printf("Failed to create OIDC config: %v", err)
		http.Error(w, "Failed to create OIDC config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// updateCurrentTenantOIDC updates OIDC config for current tenant
func (rt *Router) updateCurrentTenantOIDC(w http.ResponseWriter, r *http.Request) {
	oidcID, err := strconv.Atoi(chi.URLParam(r, "oidc_id"))
	if err != nil {
		http.Error(w, "Invalid OIDC config ID", http.StatusBadRequest)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config, err := rt.oidcService.UpdateOIDCConfig(r.Context(), oidcID, updates)
	if err != nil {
		log.Printf("Failed to update OIDC config: %v", err)
		http.Error(w, "Failed to update OIDC config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// deleteCurrentTenantOIDC deletes OIDC config for current tenant
func (rt *Router) deleteCurrentTenantOIDC(w http.ResponseWriter, r *http.Request) {
	oidcID, err := strconv.Atoi(chi.URLParam(r, "oidc_id"))
	if err != nil {
		http.Error(w, "Invalid OIDC config ID", http.StatusBadRequest)
		return
	}

	err = rt.oidcService.DeleteOIDCConfig(r.Context(), oidcID)
	if err != nil {
		log.Printf("Failed to delete OIDC config: %v", err)
		http.Error(w, "Failed to delete OIDC config", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getCurrentTenantUsage returns usage for current tenant
func (rt *Router) getCurrentTenantUsage(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenancy.GetTenantFromContext(r.Context())
	if !ok {
		http.Error(w, "Tenant context not found", http.StatusInternalServerError)
		return
	}

	// TODO: Implement usage metrics
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tenant_id":     tenant.ID,
		"storage_bytes": 0,
		"bandwidth":     0,
		"repositories":  0,
	})
}
