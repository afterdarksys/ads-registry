package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	Storage       StorageConfig       `json:"storage"`
	Auth          AuthConfig          `json:"auth"`
	Logging       LoggingConfig       `json:"logging"`
	Vault         VaultConfig         `json:"vault"`
	Webhooks      []string            `json:"webhooks"`
	Queue         QueueConfig         `json:"queue"`
	Redis         RedisConfig         `json:"redis"`
	Compatibility CompatibilityConfig `json:"compatibility"`
	Peers         []PeerRegistry      `json:"peers"`
	DarkScan      DarkScanConfig      `json:"darkscan"`
}

type PeerRegistry struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
	Mode     string `json:"mode"` // "push", "pull", or "bidirectional"
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
	Driver string      `json:"driver"` // local, s3, minio, or oci
	Local  *LocalStore `json:"local,omitempty"`
	S3     *S3Store    `json:"s3,omitempty"`
	MinIO  *MinIOStore `json:"minio,omitempty"`
	OCI    *OCIStore   `json:"oci,omitempty"`
}

type LocalStore struct {
	RootDirectory string `json:"root_directory"`
}

type S3Store struct {
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Endpoint        string `json:"endpoint,omitempty"` // For custom S3-compatible endpoints
}

type MinIOStore struct {
	Endpoint        string `json:"endpoint"`         // MinIO server endpoint (e.g., "localhost:9000")
	AccessKeyID     string `json:"access_key_id"`    // MinIO access key
	SecretAccessKey string `json:"secret_access_key"` // MinIO secret key
	Bucket          string `json:"bucket"`           // Bucket name
	UseSSL          bool   `json:"use_ssl"`          // Use HTTPS (default: false for local dev)
	Region          string `json:"region"`           // Region (usually "us-east-1" for MinIO)
}

type OCIStore struct {
	Namespace       string `json:"namespace"`        // OCI Object Storage namespace
	Bucket          string `json:"bucket"`           // Bucket name
	Region          string `json:"region"`           // OCI region (e.g., "us-phoenix-1")
	CompartmentID   string `json:"compartment_id"`   // OCI compartment OCID
	TenancyID       string `json:"tenancy_id"`       // OCI tenancy OCID
	UserID          string `json:"user_id"`          // OCI user OCID
	Fingerprint     string `json:"fingerprint"`      // API key fingerprint
	PrivateKeyPath  string `json:"private_key_path"` // Path to OCI private key file
	PrivateKey      string `json:"private_key"`      // Or inline private key (base64/PEM)
	Endpoint        string `json:"endpoint"`         // Custom endpoint (optional, auto-generated from region+namespace)
}

type AuthConfig struct {
	TokenIssuer     string        `json:"token_issuer"`
	TokenService    string        `json:"token_service"`
	PrivateKey      string        `json:"private_key_path"`
	PublicKey       string        `json:"public_key_path"`
	TokenExpiration time.Duration `json:"token_expiration"` // Duration before tokens expire (default: 24h)
	OIDC            OIDCConfig    `json:"oidc"`
}

type OIDCConfig struct {
	Enabled      bool   `json:"enabled"`
	Issuer       string `json:"issuer"`        // Authentik issuer URL (e.g., "https://sso.afterdarksys.com/application/o/ads-registry/")
	ClientID     string `json:"client_id"`     // OIDC client ID
	ClientSecret string `json:"client_secret"` // OIDC client secret
	RedirectURL  string `json:"redirect_url"`  // Callback URL (e.g., "https://registry.afterdarksys.com/oauth2/callback")
	Scopes       []string `json:"scopes"`      // OIDC scopes (default: ["openid", "profile", "email"])
}

type LoggingConfig struct {
	Level         string              `json:"level"` // debug, info, warn, error (default: info)
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

type QueueConfig struct {
	Enabled            bool `json:"enabled"`              // Enable River job queue (requires PostgreSQL)
	VulnerabilityQueue int  `json:"vulnerability_queue"`  // Max workers for vulnerability scans (default: 4)
	PeriodicQueue      int  `json:"periodic_queue"`       // Max workers for periodic tasks (default: 1)
	DefaultQueue       int  `json:"default_queue"`        // Max workers for default queue (default: 2)
}

type RedisConfig struct {
	Enabled  bool   `json:"enabled"`  // Enable Redis caching
	Address  string `json:"address"`  // Redis server address (e.g., "localhost:6379")
	Password string `json:"password"` // Redis password (empty for no password)
	DB       int    `json:"db"`       // Redis database number (0-15)
	TTL      TTLConfig `json:"ttl"`   // TTL settings for different cache types
}

type TTLConfig struct {
	Manifest   int `json:"manifest"`    // Manifest metadata TTL in seconds (default: 300 = 5 min)
	Signature  int `json:"signature"`   // Signature validation TTL in seconds (default: 300 = 5 min)
	ScanReport int `json:"scan_report"` // Vulnerability scan TTL in seconds (default: 3600 = 1 hr)
	Policy     int `json:"policy"`      // Policy evaluation TTL in seconds (default: 300 = 5 min)
}

// CompatibilityConfig controls client compatibility workarounds
// Inspired by Postfix's pragmatic approach to broken mail clients
type CompatibilityConfig struct {
	// Enable the compatibility system
	Enabled bool `json:"enabled"`

	// Docker client workarounds
	DockerClientWorkarounds DockerWorkaroundsConfig `json:"docker_client_workarounds"`

	// Protocol emulation settings
	ProtocolEmulation ProtocolEmulationConfig `json:"protocol_emulation"`

	// Broken client hacks
	BrokenClientHacks BrokenClientHacksConfig `json:"broken_client_hacks"`

	// TLS compatibility
	TLSCompatibility TLSCompatibilityConfig `json:"tls_compatibility"`

	// Header workarounds
	HeaderWorkarounds HeaderWorkaroundsConfig `json:"header_workarounds"`

	// Rate limit exceptions
	RateLimitExceptions RateLimitExceptionsConfig `json:"rate_limit_exceptions"`

	// Observability settings
	Observability ObservabilityConfig `json:"observability"`
}

type DockerWorkaroundsConfig struct {
	EnableDocker29ManifestFix bool  `json:"enable_docker_29_manifest_fix"`
	ForceHTTP1ForManifests    bool  `json:"force_http1_for_manifests"`
	DisableChunkedEncoding    bool  `json:"disable_chunked_encoding"`
	MaxManifestSize           int64 `json:"max_manifest_size"`
	ExtraFlushes              int   `json:"extra_flushes"`
	HeaderWriteDelay          int   `json:"header_write_delay_ms"`
}

type ProtocolEmulationConfig struct {
	EmulateDockerRegistryV2 bool `json:"emulate_docker_registry_v2"`
	EmulateDistributionV3   bool `json:"emulate_distribution_v3"`
	ExposeOCIFeatures       bool `json:"expose_oci_features"`
	StrictMode              bool `json:"strict_mode"`
}

type BrokenClientHacksConfig struct {
	PodmanDigestWorkaround  bool   `json:"podman_digest_workaround"`
	SkopeoLayerReuse        bool   `json:"skopeo_layer_reuse"`
	CraneManifestFormat     string `json:"crane_manifest_format"`
	ContainerdContentLength bool   `json:"containerd_content_length"`
	BuildkitParallelUpload  bool   `json:"buildkit_parallel_upload"`
	NerdctlMissingHeaders   bool   `json:"nerdctl_missing_headers"`
}

type TLSCompatibilityConfig struct {
	MinTLSVersion        string   `json:"min_tls_version"`
	EnableLegacyCiphers  bool     `json:"enable_legacy_ciphers"`
	HTTP2Enabled         bool     `json:"http2_enabled"`
	ForceHTTP1ForClients []string `json:"force_http1_for_clients"`
	ALPNProtocols        []string `json:"alpn_protocols"`
}

type HeaderWorkaroundsConfig struct {
	AlwaysSendDistributionAPIVersion bool   `json:"always_send_distribution_api_version"`
	ContentTypeFixups                bool   `json:"content_type_fixups"`
	LocationHeaderFormat             string `json:"location_header_format"`
	EnableCORS                       bool   `json:"enable_cors"`
	AcceptMalformedAccept            bool   `json:"accept_malformed_accept"`
	NormalizeDigestHeader            bool   `json:"normalize_digest_header"`
}

type RateLimitExceptionsConfig struct {
	TrustedRegistries      []string `json:"trusted_registries"`
	CICDUserAgents         []string `json:"cicd_user_agents"`
	TrustedIPRanges        []string `json:"trusted_ip_ranges"`
	BypassForAuthenticated bool     `json:"bypass_for_authenticated"`
}

type ObservabilityConfig struct {
	LogWorkarounds     bool    `json:"log_workarounds"`
	LogClientDetection bool    `json:"log_client_detection"`
	EnableMetrics      bool    `json:"enable_metrics"`
	MetricsPrefix      string  `json:"metrics_prefix"`
	LogSampleRate      float64 `json:"log_sample_rate"`
	LogSuccessOnly     bool    `json:"log_success_only"`
}

// DarkScanConfig configures the DarkScan vulnerability scanning integration via darkapi.io
type DarkScanConfig struct {
	Enabled         bool   `json:"enabled"`          // Enable DarkScan vulnerability scanning
	BaseURL         string `json:"base_url"`         // DarkScan API base URL (e.g., "https://darkapi.io")
	APIKey          string `json:"api_key"`          // API key for authentication
	ScanOnPush      bool   `json:"scan_on_push"`     // Automatically scan images on push
	ScanOnPull      bool   `json:"scan_on_pull"`     // Scan images on first pull (cache miss)
	BlockOnCritical bool   `json:"block_on_critical"` // Block pulls if critical vulnerabilities found
	BlockOnHigh     bool   `json:"block_on_high"`    // Block pulls if high severity vulnerabilities found
	MaxConcurrent   int    `json:"max_concurrent"`   // Max concurrent scans (default: 5)
	Timeout         int    `json:"timeout"`          // Scan timeout in seconds (default: 300)
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

	// Queue defaults
	if cfg.Queue.Enabled {
		if cfg.Queue.VulnerabilityQueue == 0 {
			cfg.Queue.VulnerabilityQueue = 4
		}
		if cfg.Queue.PeriodicQueue == 0 {
			cfg.Queue.PeriodicQueue = 1
		}
		if cfg.Queue.DefaultQueue == 0 {
			cfg.Queue.DefaultQueue = 2
		}
	}

	// Redis defaults
	if cfg.Redis.Enabled {
		if cfg.Redis.Address == "" {
			cfg.Redis.Address = "localhost:6379"
		}
		if cfg.Redis.TTL.Manifest == 0 {
			cfg.Redis.TTL.Manifest = 300 // 5 minutes
		}
		if cfg.Redis.TTL.Signature == 0 {
			cfg.Redis.TTL.Signature = 300 // 5 minutes
		}
		if cfg.Redis.TTL.ScanReport == 0 {
			cfg.Redis.TTL.ScanReport = 3600 // 1 hour
		}
		if cfg.Redis.TTL.Policy == 0 {
			cfg.Redis.TTL.Policy = 300 // 5 minutes
		}
	}

	// Auth defaults
	if cfg.Auth.TokenExpiration == 0 {
		cfg.Auth.TokenExpiration = 24 * time.Hour // Default 24 hours
	}

	// DarkScan defaults
	if cfg.DarkScan.Enabled {
		if cfg.DarkScan.BaseURL == "" {
			cfg.DarkScan.BaseURL = "https://darkapi.io"
		}
		if cfg.DarkScan.MaxConcurrent == 0 {
			cfg.DarkScan.MaxConcurrent = 5
		}
		if cfg.DarkScan.Timeout == 0 {
			cfg.DarkScan.Timeout = 300 // 5 minutes
		}
	}

	// Compatibility defaults
	// If not explicitly configured, enable with sensible defaults
	if !cfg.Compatibility.Enabled {
		// Check if any compatibility settings were provided
		// If none, use defaults
		cfg.Compatibility.Enabled = true
	}
	if cfg.Compatibility.Enabled {
		// Docker workarounds defaults
		if cfg.Compatibility.DockerClientWorkarounds.MaxManifestSize == 0 {
			cfg.Compatibility.DockerClientWorkarounds.MaxManifestSize = 10 * 1024 * 1024 // 10MB
		}
		if cfg.Compatibility.DockerClientWorkarounds.ExtraFlushes == 0 {
			cfg.Compatibility.DockerClientWorkarounds.ExtraFlushes = 3
		}

		// Protocol emulation defaults
		cfg.Compatibility.ProtocolEmulation.EmulateDockerRegistryV2 = true
		cfg.Compatibility.ProtocolEmulation.ExposeOCIFeatures = true

		// Broken client hacks defaults
		if cfg.Compatibility.BrokenClientHacks.CraneManifestFormat == "" {
			cfg.Compatibility.BrokenClientHacks.CraneManifestFormat = "auto"
		}

		// TLS compatibility defaults
		if cfg.Compatibility.TLSCompatibility.MinTLSVersion == "" {
			cfg.Compatibility.TLSCompatibility.MinTLSVersion = "1.2"
		}
		if len(cfg.Compatibility.TLSCompatibility.ALPNProtocols) == 0 {
			cfg.Compatibility.TLSCompatibility.ALPNProtocols = []string{"h2", "http/1.1"}
		}

		// Header workarounds defaults
		if cfg.Compatibility.HeaderWorkarounds.LocationHeaderFormat == "" {
			cfg.Compatibility.HeaderWorkarounds.LocationHeaderFormat = "absolute"
		}

		// Observability defaults
		if cfg.Compatibility.Observability.MetricsPrefix == "" {
			cfg.Compatibility.Observability.MetricsPrefix = "ads_registry_compat_"
		}
		if cfg.Compatibility.Observability.LogSampleRate == 0 {
			cfg.Compatibility.Observability.LogSampleRate = 1.0
		}
	}

	return &cfg, nil
}
