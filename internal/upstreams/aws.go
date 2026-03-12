package upstreams

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
)

// AWSProvider handles AWS ECR authentication
type AWSProvider struct{}

// NewAWSProvider creates a new AWS ECR provider
func NewAWSProvider() *AWSProvider {
	return &AWSProvider{}
}

// Name returns the provider type
func (p *AWSProvider) Name() UpstreamType {
	return UpstreamTypeAWS
}

// RefreshToken fetches a new ECR authorization token
// AWS ECR tokens are valid for 12 hours
func (p *AWSProvider) RefreshToken(ctx context.Context, registry *UpstreamRegistry) (token string, expiry time.Time, err error) {
	// Create AWS config with static credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(registry.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			registry.AccessKeyID,
			registry.SecretAccessKey,
			"", // No session token
		)),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create ECR client
	client := ecr.NewFromConfig(cfg)

	// Get authorization token
	result, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get ECR auth token: %w", err)
	}

	if len(result.AuthorizationData) == 0 {
		return "", time.Time{}, fmt.Errorf("no authorization data returned from ECR")
	}

	authData := result.AuthorizationData[0]

	// Decode the base64-encoded token (format is "AWS:password")
	decoded, err := base64.StdEncoding.DecodeString(aws.ToString(authData.AuthorizationToken))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode auth token: %w", err)
	}

	// Extract password (format is "AWS:password")
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", time.Time{}, fmt.Errorf("invalid token format")
	}

	password := parts[1]

	// AWS tokens expire in 12 hours
	expiryTime := aws.ToTime(authData.ExpiresAt)
	if expiryTime.IsZero() {
		// Fallback: assume 12 hours from now
		expiryTime = time.Now().Add(12 * time.Hour)
	}

	return password, expiryTime, nil
}

// ValidateCredentials checks if the AWS credentials are valid
func (p *AWSProvider) ValidateCredentials(ctx context.Context, registry *UpstreamRegistry) error {
	if registry.AccessKeyID == "" {
		return fmt.Errorf("AWS access key ID is required")
	}
	if registry.SecretAccessKey == "" {
		return fmt.Errorf("AWS secret access key is required")
	}
	if registry.Region == "" {
		return fmt.Errorf("AWS region is required")
	}

	// Try to get a token to validate credentials
	_, _, err := p.RefreshToken(ctx, registry)
	return err
}

// GetRegistryEndpoint returns the ECR registry URL
func (p *AWSProvider) GetRegistryEndpoint(registry *UpstreamRegistry, repository string) string {
	// ECR format: {account-id}.dkr.ecr.{region}.amazonaws.com/{repository}
	return fmt.Sprintf("%s/%s", registry.Endpoint, repository)
}

// NeedsRefresh returns true if the token should be refreshed
// Refresh 1 hour before expiry to be safe
func (p *AWSProvider) NeedsRefresh(registry *UpstreamRegistry) bool {
	if registry.CurrentToken == "" {
		return true
	}
	return time.Now().Add(1 * time.Hour).After(registry.TokenExpiry)
}
