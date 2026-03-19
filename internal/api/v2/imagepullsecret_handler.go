package v2

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/k8s"
)

// ImagePullSecretHandler handles ImagePullSecret generation endpoints
type ImagePullSecretHandler struct {
	db          *sql.DB
	tokenSvc    *auth.TokenService
	registryURL string
}

// NewImagePullSecretHandler creates a new ImagePullSecret handler
func NewImagePullSecretHandler(db *sql.DB, tokenSvc *auth.TokenService, registryURL string) *ImagePullSecretHandler {
	return &ImagePullSecretHandler{
		db:          db,
		tokenSvc:    tokenSvc,
		registryURL: registryURL,
	}
}

// RegisterRoutes registers the ImagePullSecret routes
func (h *ImagePullSecretHandler) RegisterRoutes(r chi.Router) {
	r.Post("/k8s/imagepullsecret", h.GenerateImagePullSecret)
	r.Post("/k8s/token", h.GenerateToken)
	r.Get("/k8s/imagepullsecret/example", h.GetExample)
}

// GenerateImagePullSecret generates a Kubernetes ImagePullSecret
// POST /api/v2/k8s/imagepullsecret
func (h *ImagePullSecretHandler) GenerateImagePullSecret(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	userID, ok := r.Context().Value("user_id").(int)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request
	var req k8s.ImagePullSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if err := k8s.ValidateImagePullSecretRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get user details
	username, email, err := h.getUserDetails(userID)
	if err != nil {
		http.Error(w, "failed to get user details", http.StatusInternalServerError)
		return
	}

	// Use provided username or fall back to authenticated user's username
	if req.Username == "" {
		req.Username = username
	}
	if req.Email == "" {
		req.Email = email
	}

	// Use provided registry or fall back to this registry
	if req.Registry == "" {
		req.Registry = h.registryURL
	}

	// Generate short-lived token
	ttl := req.TokenTTL
	if ttl == 0 {
		ttl = 1 * time.Hour // Default 1 hour
	}

	token, expiresAt, err := h.generateShortLivedToken(userID, username, ttl)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	// Generate ImagePullSecret
	secret, err := k8s.GenerateImagePullSecret(
		req.Name,
		req.Namespace,
		req.Registry,
		req.Username,
		token,
		req.Email,
	)
	if err != nil {
		http.Error(w, "failed to generate secret", http.StatusInternalServerError)
		return
	}

	// Convert to requested format
	format := req.Format
	if format == "" {
		format = "yaml"
	}

	var secretContent string
	if format == "json" {
		secretContent, err = secret.ToJSON()
	} else {
		secretContent, err = secret.ToYAML()
	}

	if err != nil {
		http.Error(w, "failed to format secret", http.StatusInternalServerError)
		return
	}

	// Generate kubectl command
	kubectlCmd := k8s.GenerateKubectlCommand(secretContent, req.Namespace)

	// Create response
	response := k8s.ImagePullSecretResponse{
		Secret:         secretContent,
		Format:         format,
		ExpiresAt:      expiresAt,
		CreatedAt:      time.Now(),
		KubectlCommand: kubectlCmd,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GenerateToken generates a short-lived token for registry authentication
// POST /api/v2/k8s/token
func (h *ImagePullSecretHandler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	userID, ok := r.Context().Value("user_id").(int)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request
	var req k8s.TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Get username
	username, _, err := h.getUserDetails(userID)
	if err != nil {
		http.Error(w, "failed to get user details", http.StatusInternalServerError)
		return
	}

	if req.Username == "" {
		req.Username = username
	}

	// Default expiration: 1 hour
	ttl := req.ExpiresIn
	if ttl == 0 {
		ttl = 1 * time.Hour
	}

	// Max TTL: 24 hours
	if ttl > 24*time.Hour {
		http.Error(w, "token TTL cannot exceed 24 hours", http.StatusBadRequest)
		return
	}

	// Generate token
	token, expiresAt, err := h.generateShortLivedToken(userID, username, ttl)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	// Create response
	response := k8s.TokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		ExpiresIn: int64(ttl.Seconds()),
		Username:  username,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetExample returns example requests and responses
// GET /api/v2/k8s/imagepullsecret/example
func (h *ImagePullSecretHandler) GetExample(w http.ResponseWriter, r *http.Request) {
	example := map[string]interface{}{
		"description": "ImagePullSecret API Examples",
		"endpoints": map[string]interface{}{
			"generate_secret": map[string]interface{}{
				"method":      "POST",
				"path":        "/api/v2/k8s/imagepullsecret",
				"description": "Generate a Kubernetes ImagePullSecret with short-lived token",
				"request_example": map[string]interface{}{
					"name":      "registry-secret",
					"namespace": "production",
					"registry":  "apps.afterdarksys.com:5005",
					"username":  "ryan",
					"email":     "ryan@afterdarksys.com",
					"token_ttl": "1h",
					"format":    "yaml",
				},
				"response_example": map[string]interface{}{
					"secret": "apiVersion: v1\nkind: Secret\n...",
					"format": "yaml",
					"expires_at": "2026-03-12T11:30:00Z",
					"created_at": "2026-03-12T10:30:00Z",
					"kubectl_command": "kubectl apply -f - <<EOF\n...\nEOF",
				},
			},
			"generate_token": map[string]interface{}{
				"method":      "POST",
				"path":        "/api/v2/k8s/token",
				"description": "Generate a short-lived token for registry authentication",
				"request_example": map[string]interface{}{
					"username":   "ryan",
					"expires_in": "3600",
				},
				"response_example": map[string]interface{}{
					"token":      "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
					"expires_at": "2026-03-12T11:30:00Z",
					"expires_in": 3600,
					"username":   "ryan",
				},
			},
		},
		"usage_examples": map[string]string{
			"curl_generate_secret": `curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/imagepullsecret \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "name": "registry-secret",
    "namespace": "production",
    "format": "yaml"
  }'`,
			"kubectl_apply": "kubectl apply -f - <<EOF\n<paste secret YAML here>\nEOF",
			"docker_login_with_token": `TOKEN=$(curl -X POST https://apps.afterdarksys.com:5005/api/v2/k8s/token \
  -H "Authorization: Bearer YOUR_TOKEN" | jq -r '.token')

docker login apps.afterdarksys.com:5005 -u ryan -p $TOKEN`,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(example)
}

// getUserDetails retrieves username and email for a user
func (h *ImagePullSecretHandler) getUserDetails(userID int) (string, string, error) {
	var username, email string
	query := `SELECT username, COALESCE(email, '') FROM users WHERE id = $1`
	err := h.db.QueryRow(query, userID).Scan(&username, &email)
	return username, email, err
}

// generateShortLivedToken generates a JWT token with expiration
func (h *ImagePullSecretHandler) generateShortLivedToken(_ int, _ string, ttl time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(ttl)

	// Generate JWT token (this would use your existing token generator)
	// For now, this is a placeholder - you'd integrate with internal/auth
	token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." // Placeholder

	// In production, use:
	// token, err := h.tokenGen.GenerateToken(userID, username, expiresAt)

	return token, expiresAt, nil
}
