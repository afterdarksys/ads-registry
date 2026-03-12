package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides Redis-backed caching with TTL support
type RedisCache struct {
	client *redis.Client
}

// Config holds Redis configuration
type Config struct {
	Address  string // Redis server address (e.g., "localhost:6379")
	Password string // Redis password (empty for no password)
	DB       int    // Redis database number (0-15)
	Enabled  bool   // Enable/disable Redis caching
}

// NewRedis creates a new Redis cache client
func NewRedis(cfg Config) (*RedisCache, error) {
	if !cfg.Enabled {
		return nil, nil // Return nil if caching is disabled
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
	}, nil
}

// Get retrieves a value from cache
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	if c == nil || c.client == nil {
		return nil, nil // Cache disabled
	}

	val, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	return val, err
}

// Set stores a value in cache with TTL
func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return nil // Cache disabled
	}

	return c.client.Set(ctx, key, value, ttl).Err()
}

// Delete removes a value from cache
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	if c == nil || c.client == nil {
		return nil // Cache disabled
	}

	return c.client.Del(ctx, key).Err()
}

// DeletePattern removes all keys matching a pattern
func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	if c == nil || c.client == nil {
		return nil // Cache disabled
	}

	iter := c.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// HealthCheck verifies Redis connectivity
func (c *RedisCache) HealthCheck(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil // Cache disabled, not an error
	}
	return c.client.Ping(ctx).Err()
}

// Cache keys for different types of data
const (
	KeyPrefixManifest      = "manifest:"      // manifest:namespace/repo:tag
	KeyPrefixSignature     = "signature:"     // signature:digest
	KeyPrefixScanReport    = "scan:"          // scan:digest:scanner
	KeyPrefixPolicyResult  = "policy:"        // policy:namespace:rule_hash
)

// BuildManifestKey creates a cache key for a manifest
func BuildManifestKey(namespace, repo, reference string) string {
	return fmt.Sprintf("%s%s/%s:%s", KeyPrefixManifest, namespace, repo, reference)
}

// BuildSignatureKey creates a cache key for signature validation
func BuildSignatureKey(digest string) string {
	return fmt.Sprintf("%s%s", KeyPrefixSignature, digest)
}

// BuildScanReportKey creates a cache key for vulnerability scan results
func BuildScanReportKey(digest, scanner string) string {
	return fmt.Sprintf("%s%s:%s", KeyPrefixScanReport, digest, scanner)
}

// BuildPolicyResultKey creates a cache key for policy evaluation results
func BuildPolicyResultKey(namespace, ruleHash string) string {
	return fmt.Sprintf("%s%s:%s", KeyPrefixPolicyResult, namespace, ruleHash)
}
