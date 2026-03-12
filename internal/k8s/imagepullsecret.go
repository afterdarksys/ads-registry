package k8s

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// ImagePullSecret represents a Kubernetes ImagePullSecret
type ImagePullSecret struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   SecretMetadata    `json:"metadata"`
	Type       string            `json:"type"`
	Data       map[string]string `json:"data"`
}

// SecretMetadata represents the metadata section of a Secret
type SecretMetadata struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// DockerConfig represents the .dockerconfigjson structure
type DockerConfig struct {
	Auths map[string]AuthConfig `json:"auths"`
}

// AuthConfig represents authentication configuration for a registry
type AuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
	Auth     string `json:"auth"`
}

// GenerateImagePullSecret creates a Kubernetes ImagePullSecret
func GenerateImagePullSecret(name, namespace, registry, username, password, email string) (*ImagePullSecret, error) {
	// Create auth string (base64 encoded username:password)
	authString := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	// Create docker config
	dockerConfig := DockerConfig{
		Auths: map[string]AuthConfig{
			registry: {
				Username: username,
				Password: password,
				Email:    email,
				Auth:     authString,
			},
		},
	}

	// Marshal to JSON
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal docker config: %w", err)
	}

	// Base64 encode the entire config
	encodedConfig := base64.StdEncoding.EncodeToString(dockerConfigJSON)

	// Create the secret
	secret := &ImagePullSecret{
		APIVersion: "v1",
		Kind:       "Secret",
		Metadata: SecretMetadata{
			Name:      name,
			Namespace: namespace,
		},
		Type: "kubernetes.io/dockerconfigjson",
		Data: map[string]string{
			".dockerconfigjson": encodedConfig,
		},
	}

	return secret, nil
}

// ToYAML converts the ImagePullSecret to YAML format
func (s *ImagePullSecret) ToYAML() (string, error) {
	// For simplicity, we'll use JSON-compatible YAML
	// In production, use gopkg.in/yaml.v3

	yaml := fmt.Sprintf(`apiVersion: %s
kind: %s
metadata:
  name: %s`, s.APIVersion, s.Kind, s.Metadata.Name)

	if s.Metadata.Namespace != "" {
		yaml += fmt.Sprintf("\n  namespace: %s", s.Metadata.Namespace)
	}

	if len(s.Metadata.Labels) > 0 {
		yaml += "\n  labels:"
		for k, v := range s.Metadata.Labels {
			yaml += fmt.Sprintf("\n    %s: %s", k, v)
		}
	}

	yaml += fmt.Sprintf("\ntype: %s\ndata:\n  .dockerconfigjson: %s\n",
		s.Type, s.Data[".dockerconfigjson"])

	return yaml, nil
}

// ToJSON converts the ImagePullSecret to JSON format
func (s *ImagePullSecret) ToJSON() (string, error) {
	jsonBytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal secret to JSON: %w", err)
	}
	return string(jsonBytes), nil
}

// TokenRequest represents a request for a short-lived token
type TokenRequest struct {
	Username  string        `json:"username"`
	Scopes    []string      `json:"scopes,omitempty"`
	ExpiresIn time.Duration `json:"expires_in,omitempty"` // Duration in seconds
}

// TokenResponse represents the response with a short-lived token
type TokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	ExpiresIn int64     `json:"expires_in"` // Seconds
	Username  string    `json:"username"`
}

// ImagePullSecretRequest represents a request to generate an ImagePullSecret
type ImagePullSecretRequest struct {
	Name       string        `json:"name"`
	Namespace  string        `json:"namespace,omitempty"`
	Registry   string        `json:"registry"`
	Username   string        `json:"username"`
	Email      string        `json:"email,omitempty"`
	TokenTTL   time.Duration `json:"token_ttl,omitempty"` // Duration for token validity
	Format     string        `json:"format,omitempty"`    // "yaml" or "json", default "yaml"
}

// ImagePullSecretResponse contains the generated secret and metadata
type ImagePullSecretResponse struct {
	Secret    string    `json:"secret"`     // YAML or JSON format
	Format    string    `json:"format"`     // "yaml" or "json"
	ExpiresAt time.Time `json:"expires_at"` // When the token expires
	CreatedAt time.Time `json:"created_at"`

	// kubectl command to apply
	KubectlCommand string `json:"kubectl_command,omitempty"`
}

// GenerateKubectlCommand creates the kubectl command to apply the secret
func GenerateKubectlCommand(secretYAML, namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("kubectl apply -f - <<EOF\n%s\nEOF", secretYAML)
	}
	return fmt.Sprintf("kubectl apply -f - <<EOF\n%s\nEOF", secretYAML)
}

// ValidateImagePullSecretRequest validates the request parameters
func ValidateImagePullSecretRequest(req *ImagePullSecretRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Registry == "" {
		return fmt.Errorf("registry is required")
	}
	if req.Username == "" {
		return fmt.Errorf("username is required")
	}
	if req.Format != "" && req.Format != "yaml" && req.Format != "json" {
		return fmt.Errorf("format must be 'yaml' or 'json'")
	}
	if req.TokenTTL == 0 {
		req.TokenTTL = 1 * time.Hour // Default 1 hour
	}
	if req.TokenTTL > 24*time.Hour {
		return fmt.Errorf("token TTL cannot exceed 24 hours")
	}
	return nil
}
