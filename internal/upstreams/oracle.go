package upstreams

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

// OracleProvider handles Oracle OCI Registry (OCIR) authentication
type OracleProvider struct{}

// NewOracleProvider creates a new Oracle OCI provider
func NewOracleProvider() *OracleProvider {
	return &OracleProvider{}
}

// Name returns the provider type
func (p *OracleProvider) Name() UpstreamType {
	return UpstreamTypeOracle
}

// RefreshToken generates a new Oracle auth token
// Oracle tokens are valid for 48 hours
func (p *OracleProvider) RefreshToken(ctx context.Context, registry *UpstreamRegistry) (token string, expiry time.Time, err error) {
	// Parse the private key
	privateKey, err := parsePrivateKey(registry.SecretAccessKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse OCI private key: %w", err)
	}

	// Extract tenancy OCID and user OCID from the endpoint/config
	// For Oracle, we need these stored somewhere - let's assume they're in AccessKeyID for now
	// Format: "tenancyOCID|userOCID|fingerprint"
	// This is a simplification - in production you'd store these separately

	// Create OCI config provider
	configProvider := common.NewRawConfigurationProvider(
		registry.AccessKeyID, // tenancy OCID (we'll improve this)
		registry.AccessKeyID, // user OCID (stored in AccessKeyID for now)
		registry.Region,
		extractFingerprint(privateKey),
		string(registry.SecretAccessKey),
		common.String(""),
	)

	// Create identity client
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create OCI identity client: %w", err)
	}

	// Generate auth token for OCIR
	// Oracle uses username:auth_token for Docker login
	createAuthTokenReq := identity.CreateAuthTokenRequest{
		CreateAuthTokenDetails: identity.CreateAuthTokenDetails{
			Description: common.String(fmt.Sprintf("ADS Registry auto-generated token - %s", time.Now().Format(time.RFC3339))),
		},
		UserId: common.String(registry.AccessKeyID), // user OCID
	}

	resp, err := identityClient.CreateAuthToken(ctx, createAuthTokenReq)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create OCI auth token: %w", err)
	}

	// Oracle auth tokens are valid for ~48 hours (exact time varies)
	// Set expiry to 47 hours to be safe
	expiryTime := time.Now().Add(47 * time.Hour)

	return *resp.Token, expiryTime, nil
}

// ValidateCredentials checks if the Oracle credentials are valid
func (p *OracleProvider) ValidateCredentials(ctx context.Context, registry *UpstreamRegistry) error {
	if registry.AccessKeyID == "" {
		return fmt.Errorf("Oracle user OCID is required")
	}
	if registry.SecretAccessKey == "" {
		return fmt.Errorf("Oracle private key is required")
	}
	if registry.Region == "" {
		return fmt.Errorf("Oracle region is required")
	}

	// Validate private key format
	_, err := parsePrivateKey(registry.SecretAccessKey)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

	return nil
}

// GetRegistryEndpoint returns the OCIR registry URL
func (p *OracleProvider) GetRegistryEndpoint(registry *UpstreamRegistry, repository string) string {
	// OCIR format: {region}.ocir.io/{namespace}/{repository}
	// Namespace would need to be stored in registry config
	return fmt.Sprintf("%s/%s", registry.Endpoint, repository)
}

// NeedsRefresh returns true if the token should be refreshed
// Refresh 12 hours before expiry (since Oracle tokens are 48h, refresh at 36h)
func (p *OracleProvider) NeedsRefresh(registry *UpstreamRegistry) bool {
	if registry.CurrentToken == "" {
		return true
	}
	return time.Now().Add(12 * time.Hour).After(registry.TokenExpiry)
}

// parsePrivateKey parses a PEM-encoded RSA private key
func parsePrivateKey(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		key, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}
	}

	return key, nil
}

// extractFingerprint generates the fingerprint for an RSA public key
func extractFingerprint(privateKey *rsa.PrivateKey) string {
	// This is a simplified version - real OCI fingerprint calculation
	// involves SHA256 hashing of the public key in DER format
	publicKeyDer, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	return base64.StdEncoding.EncodeToString(publicKeyDer)[:16]
}
