package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ryan/ads-registry/internal/config"
)

type TokenService struct {
	issuer     string
	service    string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

type Claims struct {
	jwt.RegisteredClaims
	Access []AccessEntry `json:"access"`
}

type AccessEntry struct {
	Type    string   `json:"type"`    // e.g., "repository"
	Name    string   `json:"name"`    // e.g., "library/ubuntu"
	Actions []string `json:"actions"` // e.g., ["pull", "push"]
}

func NewTokenService(cfg config.AuthConfig) (*TokenService, error) {
	var privateKey *rsa.PrivateKey
	var publicKey *rsa.PublicKey
	var err error

	// Load private key if specified
	if cfg.PrivateKey != "" {
		keyData, err := os.ReadFile(cfg.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}
		privateKey, err = jwt.ParseRSAPrivateKeyFromPEM(keyData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		// Derive public key from private key
		publicKey = &privateKey.PublicKey
	}

	// Load public key if specified separately
	if cfg.PublicKey != "" {
		keyData, err := os.ReadFile(cfg.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to read public key: %w", err)
		}
		publicKey, err = jwt.ParseRSAPublicKeyFromPEM(keyData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key: %w", err)
		}
	}

	// If no keys provided, generate in-memory keys for development
	if privateKey == nil && publicKey == nil {
		fmt.Println("WARNING: No RSA keys configured. Generating ephemeral keys for development only.")
		fmt.Println("WARNING: DO NOT USE IN PRODUCTION - tokens will be invalid after restart.")
		privateKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
		}
		publicKey = &privateKey.PublicKey
	}

	return &TokenService{
		issuer:     cfg.TokenIssuer,
		service:    cfg.TokenService,
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

// GenerateToken creates a signed JWT for the given subject and granted scopes.
func (t *TokenService) GenerateToken(subject string, access []AccessEntry) (string, error) {
	if t.privateKey == nil {
		return "", fmt.Errorf("private key not initialized - cannot generate tokens")
	}

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    t.issuer,
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
		Access: access,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(t.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

func (t *TokenService) ParseToken(tokenString string) (*Claims, error) {
	if t.publicKey == nil {
		return nil, fmt.Errorf("public key not initialized - cannot validate tokens")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return t.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		// Validate expiration
		if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
			return nil, fmt.Errorf("token expired")
		}
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}
