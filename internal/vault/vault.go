package vault

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles HashiCorp Vault API interactions
type Client struct {
	address   string
	token     string
	mountPath string
	httpClient *http.Client
}

// NewClient creates a new Vault client
func NewClient(address, token, mountPath string) *Client {
	return &Client{
		address:   address,
		token:     token,
		mountPath: mountPath,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetJWTKeys retrieves RSA private and public keys from Vault
func (c *Client) GetJWTKeys(keyPath string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	// Read secret from Vault KV v2
	url := fmt.Sprintf("%s/v1/%s/data/%s", c.address, c.mountPath, keyPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to Vault: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("vault returned status %d: %s", resp.StatusCode, string(body))
	}

	var vaultResp struct {
		Data struct {
			Data map[string]interface{} `json:"data"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract PEM-encoded keys
	privateKeyPEM, ok := vaultResp.Data.Data["private_key"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("private_key not found in vault secret")
	}

	publicKeyPEM, ok := vaultResp.Data.Data["public_key"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("public_key not found in vault secret")
	}

	// Parse private key
	privateKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Parse public key
	publicKey, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return privateKey, publicKey, nil
}

// parsePrivateKey parses PEM-encoded RSA private key
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 format
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA private key")
	}

	return rsaKey, nil
}

// parsePublicKey parses PEM-encoded RSA public key
func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Try PKCS1 format
		key, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key: %w", err)
		}
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA public key")
	}

	return rsaKey, nil
}

// HealthCheck verifies connectivity to Vault
func (c *Client) HealthCheck() error {
	url := fmt.Sprintf("%s/v1/sys/health", c.address)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Vault: %w", err)
	}
	defer resp.Body.Close()

	// Vault health endpoint returns 200 when initialized and unsealed
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vault health check failed with status %d", resp.StatusCode)
	}

	return nil
}
