package compat

import (
	"regexp"
	"time"
)

// Config defines the complete compatibility configuration structure
// Following Postfix's philosophy of pragmatic workarounds for broken clients
type Config struct {
	// Enabled controls whether the compatibility system is active
	Enabled bool `json:"enabled"`

	// Docker-specific client workarounds
	DockerClientWorkarounds DockerWorkarounds `json:"docker_client_workarounds"`

	// Protocol emulation and compatibility modes
	ProtocolEmulation ProtocolEmulation `json:"protocol_emulation"`

	// Workarounds for known broken client behaviors
	BrokenClientHacks BrokenClientHacks `json:"broken_client_hacks"`

	// TLS and HTTP version compatibility
	TLSCompatibility TLSCompatibility `json:"tls_compatibility"`

	// HTTP header workarounds
	HeaderWorkarounds HeaderWorkarounds `json:"header_workarounds"`

	// Rate limiting exceptions for trusted clients
	RateLimitExceptions RateLimitExceptions `json:"rate_limit_exceptions"`

	// Observability settings
	Observability ObservabilityConfig `json:"observability"`
}

// DockerWorkarounds contains fixes for Docker client bugs
type DockerWorkarounds struct {
	// Enable workaround for Docker 29.2.0 manifest upload bug
	// Issue: "short copy: wrote 1 of 2858" error on manifest PUT
	// Applies to: Docker 29.x series
	// Removal target: Docker 30.0.0+
	EnableDocker29ManifestFix bool `json:"enable_docker_29_manifest_fix"`

	// Force HTTP/1.1 for manifest operations (disables HTTP/2)
	// Helps with Docker clients that have HTTP/2 chunked transfer bugs
	ForceHTTP1ForManifests bool `json:"force_http1_for_manifests"`

	// Disable chunked transfer encoding for manifest uploads
	// Some Docker versions incorrectly handle chunked responses
	DisableChunkedEncoding bool `json:"disable_chunked_encoding"`

	// Maximum manifest size before applying buffering workaround (bytes)
	// Default: 10MB. Set to 0 to buffer all manifests.
	MaxManifestSize int64 `json:"max_manifest_size"`

	// Number of extra Flush() calls after writing manifest response
	// Docker 29.x needs extra flushes to avoid "short copy" errors
	// Default: 3
	ExtraFlushes int `json:"extra_flushes"`

	// Add artificial delay after header write (milliseconds)
	// Helps some Docker clients that read headers too fast
	// Default: 0 (disabled)
	HeaderWriteDelay int `json:"header_write_delay_ms"`
}

// ProtocolEmulation controls protocol compatibility modes
type ProtocolEmulation struct {
	// Emulate Docker Registry v2 API quirks
	// Some clients expect non-standard v2 behaviors
	EmulateDockerRegistryV2 bool `json:"emulate_docker_registry_v2"`

	// Emulate Distribution v3 API
	// Future-proofing for OCI Distribution v3 spec
	EmulateDistributionV3 bool `json:"emulate_distribution_v3"`

	// Expose OCI-specific features
	// Enable OCI artifact support, referrers API, etc.
	ExposeOCIFeatures bool `json:"expose_oci_features"`

	// Strict standards mode (disable all workarounds)
	// For testing compliance with pure OCI spec
	StrictMode bool `json:"strict_mode"`
}

// BrokenClientHacks contains workarounds for specific client bugs
type BrokenClientHacks struct {
	// Podman digest format workaround
	// Issue: Podman sometimes sends "sha256-" instead of "sha256:"
	// Applies to: Podman 3.x and earlier
	PodmanDigestWorkaround bool `json:"podman_digest_workaround"`

	// Skopeo layer cross-mount workaround
	// Issue: Skopeo attempts cross-mount without proper detection
	// Applies to: Skopeo 1.x series
	SkopeoLayerReuse bool `json:"skopeo_layer_reuse"`

	// Crane manifest format override
	// Issue: Crane sometimes requests wrong manifest format
	// Values: "auto" (detect), "docker" (force Docker), "oci" (force OCI)
	CraneManifestFormat string `json:"crane_manifest_format"`

	// Containerd missing Content-Length workaround
	// Issue: Containerd 1.6.x doesn't handle missing Content-Length
	// Applies to: Containerd 1.6.x
	ContainerdContentLength bool `json:"containerd_content_length"`

	// Buildkit parallel upload handling
	// Issue: BuildKit sends parallel uploads without proper coordination
	// Applies to: BuildKit 0.10.x and earlier
	BuildkitParallelUpload bool `json:"buildkit_parallel_upload"`

	// Nerdctl missing headers workaround
	// Issue: Nerdctl sometimes omits required headers
	NerdctlMissingHeaders bool `json:"nerdctl_missing_headers"`
}

// TLSCompatibility controls TLS and HTTP version negotiation
type TLSCompatibility struct {
	// Minimum TLS version for registry connections
	// Values: "1.0", "1.1", "1.2", "1.3"
	// Default: "1.2"
	MinTLSVersion string `json:"min_tls_version"`

	// Enable legacy cipher suites for old clients
	// Security warning: Only enable if absolutely necessary
	EnableLegacyCiphers bool `json:"enable_legacy_ciphers"`

	// Enable HTTP/2 support
	// Disable if clients have HTTP/2 bugs
	HTTP2Enabled bool `json:"http2_enabled"`

	// Force HTTP/1.1 for specific client patterns (regex)
	// Example: ["Docker/29\\..*", "containerd/1\\.6\\..*"]
	ForceHTTP1ForClients []string `json:"force_http1_for_clients"`

	// Compiled regex patterns (internal use)
	forceHTTP1Patterns []*regexp.Regexp `json:"-"`

	// ALPN protocol preferences
	// Default: ["h2", "http/1.1"]
	ALPNProtocols []string `json:"alpn_protocols"`

	// === NUCLEAR TLS OPTIONS ===
	// These are "ugly but necessary" options for dealing with broken TLS setups
	// Inspired by OpenSSL's SSL_CTX_set_verify_depth

	// Enable certificate path validation depth control
	// When true, limits how far up the chain we validate certificates
	// Use cases:
	//   - Clients with incomplete intermediate certificate chains
	//   - Corporate MITM proxies with custom CA hierarchies
	//   - Cross-signed certificates creating validation loops
	//   - Clients that send too many intermediates
	// Default: false (validate full chain to root)
	// Security warning: Only enable if you have broken clients you can't fix
	EnableCertificatePathValidation bool `json:"enable_certificate_path_validation"`

	// Certificate validation depth (how many certificates to check)
	// 0 = only leaf certificate (highly insecure, DO NOT USE)
	// 1 = leaf + 1 intermediate
	// 2 = leaf + 2 intermediates (reasonable for broken clients)
	// 3 = leaf + 3 intermediates
	// -1 = unlimited (validate to root, default behavior)
	//
	// Example scenarios:
	//   depth=1: Client cert → Intermediate CA (stop here, don't check root)
	//   depth=2: Client cert → Intermediate CA → Sub-CA (stop here)
	//
	// Only used if EnableCertificatePathValidation is true
	// Default: -1 (unlimited)
	// Security warning: Lower values = less security, more compatibility
	CertificatePathValidationDepth int `json:"certificate_path_validation_depth"`

	// Skip verification of certificate chain completeness
	// When true, allows clients to omit intermediate certificates
	// Registry will attempt to build chain from known intermediates
	// Default: false
	// Security warning: This trusts the registry's intermediate cache
	AllowIncompleteChains bool `json:"allow_incomplete_chains"`

	// Accept self-signed certificates from specific clients (regex patterns)
	// Example: ["docker-dev-.*", "localhost:.*"]
	// Default: [] (empty, no exceptions)
	// Security warning: Only for development/testing environments
	AcceptSelfSignedFromClients []string `json:"accept_self_signed_from_clients"`

	// Compiled regex patterns for self-signed exceptions (internal use)
	acceptSelfSignedPatterns []*regexp.Regexp `json:"-"`
}

// HeaderWorkarounds controls HTTP header compatibility fixes
type HeaderWorkarounds struct {
	// Always send Docker-Distribution-API-Version header
	// Required by some old Docker clients
	AlwaysSendDistributionAPIVersion bool `json:"always_send_distribution_api_version"`

	// Fix Content-Type headers for compatibility
	// Normalizes application/vnd.docker.* vs application/vnd.oci.*
	ContentTypeFixups bool `json:"content_type_fixups"`

	// Location header format
	// Values: "absolute" (full URL), "relative" (path only)
	// Default: "absolute"
	LocationHeaderFormat string `json:"location_header_format"`

	// Add CORS headers for web-based clients
	EnableCORS bool `json:"enable_cors"`

	// Accept malformed Accept headers
	// Some clients send invalid Accept header formats
	AcceptMalformedAccept bool `json:"accept_malformed_accept"`

	// Normalize Docker-Content-Digest header
	// Some clients expect specific digest formats
	NormalizeDigestHeader bool `json:"normalize_digest_header"`
}

// RateLimitExceptions defines rate limit exemptions for trusted clients
type RateLimitExceptions struct {
	// Trusted registry hostnames (for cross-registry pulls)
	// Example: ["gcr.io", "docker.io", "quay.io"]
	TrustedRegistries []string `json:"trusted_registries"`

	// CI/CD user agents to exempt from rate limits
	// Example: ["GitLab-Runner/.*", "GitHub-Actions/.*"]
	CICDUserAgents []string `json:"cicd_user_agents"`

	// Compiled regex patterns (internal use)
	cicdPatterns []*regexp.Regexp `json:"-"`

	// Trusted IP ranges (CIDR notation)
	// Example: ["10.0.0.0/8", "192.168.0.0/16"]
	TrustedIPRanges []string `json:"trusted_ip_ranges"`

	// Bypass rate limits for authenticated users
	BypassForAuthenticated bool `json:"bypass_for_authenticated"`
}

// ObservabilityConfig controls compatibility system observability
type ObservabilityConfig struct {
	// Log all workaround activations
	LogWorkarounds bool `json:"log_workarounds"`

	// Log client detection details
	LogClientDetection bool `json:"log_client_detection"`

	// Export Prometheus metrics
	EnableMetrics bool `json:"enable_metrics"`

	// Metrics prefix
	// Default: "ads_registry_compat_"
	MetricsPrefix string `json:"metrics_prefix"`

	// Sample rate for detailed logging (0.0 to 1.0)
	// 1.0 = log everything, 0.1 = log 10% of requests
	// Default: 1.0
	LogSampleRate float64 `json:"log_sample_rate"`

	// Log successful workarounds or only failures
	LogSuccessOnly bool `json:"log_success_only"`
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		DockerClientWorkarounds: DockerWorkarounds{
			EnableDocker29ManifestFix: true,
			ForceHTTP1ForManifests:    true,
			DisableChunkedEncoding:    false,
			MaxManifestSize:           10 * 1024 * 1024, // 10MB
			ExtraFlushes:              3,
			HeaderWriteDelay:          0,
		},
		ProtocolEmulation: ProtocolEmulation{
			EmulateDockerRegistryV2: true,
			EmulateDistributionV3:   false,
			ExposeOCIFeatures:       true,
			StrictMode:              false,
		},
		BrokenClientHacks: BrokenClientHacks{
			PodmanDigestWorkaround:  true,
			SkopeoLayerReuse:        true,
			CraneManifestFormat:     "auto",
			ContainerdContentLength: true,
			BuildkitParallelUpload:  false,
			NerdctlMissingHeaders:   true,
		},
		TLSCompatibility: TLSCompatibility{
			MinTLSVersion:        "1.2",
			EnableLegacyCiphers:  false,
			HTTP2Enabled:         true,
			ForceHTTP1ForClients: []string{`(?i)docker/v?29\..*`},
			ALPNProtocols:        []string{"h2", "http/1.1"},
			// Nuclear TLS options (disabled by default for security)
			EnableCertificatePathValidation: false, // Only enable for broken clients
			CertificatePathValidationDepth:  -1,    // -1 = unlimited (validate to root)
			AllowIncompleteChains:           false, // Don't trust incomplete chains
			AcceptSelfSignedFromClients:     []string{}, // No self-signed exceptions
		},
		HeaderWorkarounds: HeaderWorkarounds{
			AlwaysSendDistributionAPIVersion: true,
			ContentTypeFixups:                true,
			LocationHeaderFormat:             "absolute",
			EnableCORS:                       false,
			AcceptMalformedAccept:            true,
			NormalizeDigestHeader:            true,
		},
		RateLimitExceptions: RateLimitExceptions{
			TrustedRegistries:      []string{},
			CICDUserAgents:         []string{"GitLab-Runner/.*", "GitHub-Actions/.*"},
			TrustedIPRanges:        []string{},
			BypassForAuthenticated: false,
		},
		Observability: ObservabilityConfig{
			LogWorkarounds:     true,
			LogClientDetection: true,
			EnableMetrics:      true,
			MetricsPrefix:      "ads_registry_compat_",
			LogSampleRate:      1.0,
			LogSuccessOnly:     false,
		},
	}
}

// Validate checks the configuration for errors and compiles regex patterns
func (c *Config) Validate() error {
	// Compile TLS compatibility patterns
	if len(c.TLSCompatibility.ForceHTTP1ForClients) > 0 {
		patterns := make([]*regexp.Regexp, 0, len(c.TLSCompatibility.ForceHTTP1ForClients))
		for _, pattern := range c.TLSCompatibility.ForceHTTP1ForClients {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return err
			}
			patterns = append(patterns, re)
		}
		c.TLSCompatibility.forceHTTP1Patterns = patterns
	}

	// Compile self-signed certificate exception patterns
	if len(c.TLSCompatibility.AcceptSelfSignedFromClients) > 0 {
		patterns := make([]*regexp.Regexp, 0, len(c.TLSCompatibility.AcceptSelfSignedFromClients))
		for _, pattern := range c.TLSCompatibility.AcceptSelfSignedFromClients {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return err
			}
			patterns = append(patterns, re)
		}
		c.TLSCompatibility.acceptSelfSignedPatterns = patterns
	}

	// Compile CI/CD user agent patterns
	if len(c.RateLimitExceptions.CICDUserAgents) > 0 {
		patterns := make([]*regexp.Regexp, 0, len(c.RateLimitExceptions.CICDUserAgents))
		for _, pattern := range c.RateLimitExceptions.CICDUserAgents {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return err
			}
			patterns = append(patterns, re)
		}
		c.RateLimitExceptions.cicdPatterns = patterns
	}

	return nil
}

// ShouldAcceptSelfSigned checks if a client is allowed to use self-signed certificates
func (c *TLSCompatibility) ShouldAcceptSelfSigned(clientIdentifier string) bool {
	if !c.EnableCertificatePathValidation {
		return false // Must be explicitly enabled
	}

	for _, pattern := range c.acceptSelfSignedPatterns {
		if pattern.MatchString(clientIdentifier) {
			return true
		}
	}
	return false
}

// GetValidationDepth returns the certificate validation depth to use
// Returns -1 for unlimited (standard behavior), or the configured depth
func (c *TLSCompatibility) GetValidationDepth() int {
	if !c.EnableCertificatePathValidation {
		return -1 // Unlimited when feature disabled
	}
	return c.CertificatePathValidationDepth
}

// ShouldForceHTTP1 checks if a client should be forced to HTTP/1.1
func (c *TLSCompatibility) ShouldForceHTTP1(userAgent string) bool {
	for _, pattern := range c.forceHTTP1Patterns {
		if pattern.MatchString(userAgent) {
			return true
		}
	}
	return false
}

// IsCICDClient checks if a user agent matches CI/CD patterns
func (r *RateLimitExceptions) IsCICDClient(userAgent string) bool {
	for _, pattern := range r.cicdPatterns {
		if pattern.MatchString(userAgent) {
			return true
		}
	}
	return false
}

// WorkaroundActivation represents a single workaround being applied
type WorkaroundActivation struct {
	Name        string    `json:"name"`
	Reason      string    `json:"reason"`
	ClientName  string    `json:"client_name"`
	ClientVer   string    `json:"client_version"`
	RequestPath string    `json:"request_path"`
	Timestamp   time.Time `json:"timestamp"`
}
