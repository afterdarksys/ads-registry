package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Storage  StorageConfig  `json:"storage"`
	Auth     AuthConfig     `json:"auth"`
	Logging  LoggingConfig  `json:"logging"`
	Vault    VaultConfig    `json:"vault"`
	Webhooks []string       `json:"webhooks"`
}

type ServerConfig struct {
	Address           string        `json:"address"`
	Port              int           `json:"port"`
	ReadTimeout       time.Duration `json:"read_timeout"`
	WriteTimeout      time.Duration `json:"write_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout"`
	ReadHeaderTimeout time.Duration `json:"read_header_timeout"`
	TLS               TLSConfig     `json:"tls"`
}

type TLSConfig struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

type DatabaseConfig struct {
	Driver          string        `json:"driver"` // sqlite3 or postgres
	DSN             string        `json:"dsn"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

type StorageConfig struct {
	Driver string      `json:"driver"` // local or s3
	Local  *LocalStore `json:"local,omitempty"`
	S3     *S3Store    `json:"s3,omitempty"`
}

type LocalStore struct {
	RootDirectory string `json:"root_directory"`
}

type S3Store struct {
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Endpoint        string `json:"endpoint,omitempty"` // For Minio/Custom S3
}

type AuthConfig struct {
	TokenIssuer  string `json:"token_issuer"`
	TokenService string `json:"token_service"`
	PrivateKey   string `json:"private_key_path"`
	PublicKey    string `json:"public_key_path"`
}

type LoggingConfig struct {
	Syslog        SyslogConfig        `json:"syslog"`
	Elasticsearch ElasticsearchConfig `json:"elasticsearch"`
}

type SyslogConfig struct {
	Enabled  bool   `json:"enabled"`
	Server   string `json:"server"`   // "local", "tcp://host:port", "udp://host:port", "unix:///path"
	Tag      string `json:"tag"`      // Defaults to "ads-registry"
	Priority string `json:"priority"` // DEBUG, INFO, WARNING, ERROR, CRITICAL
}

type ElasticsearchConfig struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint"` // e.g., "http://localhost:9200"
	Index    string `json:"index"`    // Base index name, will append date
	Username string `json:"username"`
	Password string `json:"password"`
}

type VaultConfig struct {
	Enabled   bool   `json:"enabled"`
	Address   string `json:"address"`    // e.g., "http://localhost:8200"
	Token     string `json:"token"`      // Vault token for authentication
	MountPath string `json:"mount_path"` // KV mount path, e.g., "secret"
	KeyPath   string `json:"key_path"`   // Path to JWT keys in Vault
}

// LoadFile reads and parses a JSON configuration file into the Config struct.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply defaults
	if cfg.Server.Address == "" {
		cfg.Server.Address = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 5005
	}

	// Env variables overrides
	if envDSN := os.Getenv("REGISTRY_DB_DSN"); envDSN != "" {
		cfg.Database.DSN = envDSN
	}
	if envCert := os.Getenv("REGISTRY_TLS_CERT"); envCert != "" {
		cfg.Server.TLS.CertFile = envCert
		cfg.Server.TLS.Enabled = true
	}
	if envKey := os.Getenv("REGISTRY_TLS_KEY"); envKey != "" {
		cfg.Server.TLS.KeyFile = envKey
		cfg.Server.TLS.Enabled = true
	}
	if cfg.Server.TLS.Enabled && cfg.Server.TLS.Port == 0 {
		cfg.Server.TLS.Port = 5006
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite3"
		cfg.Database.DSN = "data/registry.db"
	}

	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 10 * time.Second
		cfg.Server.WriteTimeout = 10 * time.Second
		cfg.Server.IdleTimeout = 60 * time.Second
		cfg.Server.ReadHeaderTimeout = 5 * time.Second
	}

	if len(cfg.Webhooks) == 0 {
		cfg.Webhooks = []string{"http://localhost:8080/webhooks"}
	}

	return &cfg, nil
}
