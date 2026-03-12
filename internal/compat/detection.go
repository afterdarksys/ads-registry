package compat

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// ClientInfo contains detected client metadata
type ClientInfo struct {
	// Detected client name (docker, containerd, podman, etc.)
	Name string

	// Semantic version (e.g., "29.2.0")
	Version string

	// Major, minor, patch components
	VersionMajor int
	VersionMinor int
	VersionPatch int

	// Client protocol/ecosystem
	Protocol string // "docker", "containerd", "oci", "unknown"

	// HTTP protocol version
	HTTPVersion string // "HTTP/1.1", "HTTP/2.0"

	// Raw User-Agent string
	UserAgent string

	// Detected capabilities
	Capabilities []string

	// Active workarounds for this client
	Workarounds []string

	// Additional metadata from fingerprinting
	Metadata map[string]string
}

// Context key for storing ClientInfo
type clientInfoKey struct{}

// WithClientInfo adds ClientInfo to the request context
func WithClientInfo(ctx context.Context, info *ClientInfo) context.Context {
	return context.WithValue(ctx, clientInfoKey{}, info)
}

// GetClientInfo retrieves ClientInfo from the request context
func GetClientInfo(ctx context.Context) *ClientInfo {
	if info, ok := ctx.Value(clientInfoKey{}).(*ClientInfo); ok {
		return info
	}
	return nil
}

// ClientDetector handles client detection and fingerprinting
type ClientDetector struct {
	// User-Agent parsing patterns
	patterns []*clientPattern
}

// clientPattern defines a regex pattern for matching user agents
type clientPattern struct {
	regex    *regexp.Regexp
	name     string
	protocol string
}

// NewClientDetector creates a new client detector
func NewClientDetector() *ClientDetector {
	patterns := []*clientPattern{
		// Docker clients
		{
			regex:    regexp.MustCompile(`(?i)docker/(\d+)\.(\d+)\.(\d+)`),
			name:     "docker",
			protocol: "docker",
		},
		{
			regex:    regexp.MustCompile(`(?i)docker/(\d+)\.(\d+)`),
			name:     "docker",
			protocol: "docker",
		},

		// Containerd
		{
			regex:    regexp.MustCompile(`(?i)containerd/(\d+)\.(\d+)\.(\d+)`),
			name:     "containerd",
			protocol: "containerd",
		},
		{
			regex:    regexp.MustCompile(`(?i)containerd/v?(\d+)\.(\d+)`),
			name:     "containerd",
			protocol: "containerd",
		},

		// Podman
		{
			regex:    regexp.MustCompile(`(?i)podman/(\d+)\.(\d+)\.(\d+)`),
			name:     "podman",
			protocol: "docker",
		},
		{
			regex:    regexp.MustCompile(`(?i)podman/v?(\d+)\.(\d+)`),
			name:     "podman",
			protocol: "docker",
		},

		// Skopeo
		{
			regex:    regexp.MustCompile(`(?i)skopeo/(\d+)\.(\d+)\.(\d+)`),
			name:     "skopeo",
			protocol: "oci",
		},
		{
			regex:    regexp.MustCompile(`(?i)skopeo/v?(\d+)\.(\d+)`),
			name:     "skopeo",
			protocol: "oci",
		},

		// Crane (Google)
		{
			regex:    regexp.MustCompile(`(?i)go-containerregistry/v(\d+)\.(\d+)\.(\d+)`),
			name:     "crane",
			protocol: "oci",
		},
		{
			regex:    regexp.MustCompile(`(?i)crane/v?(\d+)\.(\d+)`),
			name:     "crane",
			protocol: "oci",
		},

		// Buildkit
		{
			regex:    regexp.MustCompile(`(?i)buildkit/(\d+)\.(\d+)\.(\d+)`),
			name:     "buildkit",
			protocol: "docker",
		},
		{
			regex:    regexp.MustCompile(`(?i)buildkit/v?(\d+)\.(\d+)`),
			name:     "buildkit",
			protocol: "docker",
		},

		// Nerdctl
		{
			regex:    regexp.MustCompile(`(?i)nerdctl/(\d+)\.(\d+)\.(\d+)`),
			name:     "nerdctl",
			protocol: "containerd",
		},

		// Kubernetes kubelet (image pulls)
		{
			regex:    regexp.MustCompile(`(?i)kubelet/v?(\d+)\.(\d+)\.(\d+)`),
			name:     "kubelet",
			protocol: "containerd",
		},

		// Harbor registry replication
		{
			regex:    regexp.MustCompile(`(?i)harbor-registry-client`),
			name:     "harbor",
			protocol: "docker",
		},

		// Generic Go client
		{
			regex:    regexp.MustCompile(`(?i)Go-http-client/(\d+)\.(\d+)`),
			name:     "go-http-client",
			protocol: "unknown",
		},

		// Curl (for testing)
		{
			regex:    regexp.MustCompile(`(?i)curl/(\d+)\.(\d+)\.(\d+)`),
			name:     "curl",
			protocol: "unknown",
		},
	}

	return &ClientDetector{
		patterns: patterns,
	}
}

// DetectClient analyzes an HTTP request and returns ClientInfo
func (d *ClientDetector) DetectClient(r *http.Request) *ClientInfo {
	userAgent := r.Header.Get("User-Agent")

	info := &ClientInfo{
		UserAgent:   userAgent,
		HTTPVersion: r.Proto,
		Metadata:    make(map[string]string),
		Workarounds: make([]string, 0),
	}

	// Parse User-Agent
	if userAgent != "" {
		d.parseUserAgent(info, userAgent)
	}

	// Fallback: try to detect from other headers
	if info.Name == "" {
		d.fingerprintFromHeaders(info, r)
	}

	// Default to unknown
	if info.Name == "" {
		info.Name = "unknown"
		info.Protocol = "unknown"
	}

	// Detect capabilities
	d.detectCapabilities(info, r)

	return info
}

// parseUserAgent extracts client information from User-Agent header
func (d *ClientDetector) parseUserAgent(info *ClientInfo, userAgent string) {
	for _, pattern := range d.patterns {
		matches := pattern.regex.FindStringSubmatch(userAgent)
		if len(matches) > 0 {
			info.Name = pattern.name
			info.Protocol = pattern.protocol

			// Parse version components
			if len(matches) >= 2 {
				info.VersionMajor, _ = strconv.Atoi(matches[1])
			}
			if len(matches) >= 3 {
				info.VersionMinor, _ = strconv.Atoi(matches[2])
			}
			if len(matches) >= 4 {
				info.VersionPatch, _ = strconv.Atoi(matches[3])
			}

			// Construct full version string
			if info.VersionPatch > 0 {
				info.Version = matches[1] + "." + matches[2] + "." + matches[3]
			} else if info.VersionMinor > 0 {
				info.Version = matches[1] + "." + matches[2]
			} else {
				info.Version = matches[1]
			}

			return
		}
	}
}

// fingerprintFromHeaders attempts to detect client from other headers
func (d *ClientDetector) fingerprintFromHeaders(info *ClientInfo, r *http.Request) {
	// Docker-specific headers
	if r.Header.Get("X-Docker-Token") != "" || r.Header.Get("X-Registry-Auth") != "" {
		info.Name = "docker"
		info.Protocol = "docker"
		info.Version = "unknown"
		return
	}

	// Distribution API version header (generic Docker-compatible)
	if apiVersion := r.Header.Get("Docker-Distribution-API-Version"); apiVersion != "" {
		info.Metadata["api-version"] = apiVersion
		info.Protocol = "docker"
	}

	// OCI artifact media types indicate OCI-native client
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/vnd.oci.image") {
		if info.Protocol == "" {
			info.Protocol = "oci"
		}
		info.Capabilities = append(info.Capabilities, "oci-artifacts")
	}
}

// detectCapabilities determines client capabilities from request
func (d *ClientDetector) detectCapabilities(info *ClientInfo, r *http.Request) {
	accept := r.Header.Get("Accept")

	// OCI Image Spec support
	if strings.Contains(accept, "application/vnd.oci.image.manifest.v1+json") {
		info.Capabilities = append(info.Capabilities, "oci-image")
	}

	// Docker manifest v2 schema 2
	if strings.Contains(accept, "application/vnd.docker.distribution.manifest.v2+json") {
		info.Capabilities = append(info.Capabilities, "docker-v2-schema2")
	}

	// Docker manifest list (multi-arch)
	if strings.Contains(accept, "application/vnd.docker.distribution.manifest.list.v2+json") {
		info.Capabilities = append(info.Capabilities, "manifest-list")
	}

	// OCI Image Index (multi-arch)
	if strings.Contains(accept, "application/vnd.oci.image.index.v1+json") {
		info.Capabilities = append(info.Capabilities, "oci-index")
	}

	// HTTP/2 support
	if r.ProtoMajor == 2 {
		info.Capabilities = append(info.Capabilities, "http2")
	}

	// Compression support
	acceptEncoding := r.Header.Get("Accept-Encoding")
	if strings.Contains(acceptEncoding, "gzip") {
		info.Capabilities = append(info.Capabilities, "gzip")
	}
	if strings.Contains(acceptEncoding, "zstd") {
		info.Capabilities = append(info.Capabilities, "zstd")
	}
}

// MatchesVersion checks if the client version matches a version pattern
// Patterns can be:
//   - Exact: "29.2.0"
//   - Major only: "29"
//   - Major.minor: "29.2"
//   - Wildcard: "29.*", "29.2.*"
//   - Range: ">=29.0.0,<30.0.0"
func (info *ClientInfo) MatchesVersion(pattern string) bool {
	// Exact match
	if info.Version == pattern {
		return true
	}

	// Wildcard matching
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(info.Version, prefix)
	}

	// Major version only
	if !strings.Contains(pattern, ".") {
		patternMajor, _ := strconv.Atoi(pattern)
		return info.VersionMajor == patternMajor
	}

	// Major.minor matching
	parts := strings.Split(pattern, ".")
	if len(parts) == 2 {
		major, _ := strconv.Atoi(parts[0])
		minor, _ := strconv.Atoi(parts[1])
		return info.VersionMajor == major && info.VersionMinor == minor
	}

	return false
}

// IsDockerClient returns true if this is a Docker client
func (info *ClientInfo) IsDockerClient() bool {
	return info.Name == "docker"
}

// IsPodmanClient returns true if this is a Podman client
func (info *ClientInfo) IsPodmanClient() bool {
	return info.Name == "podman"
}

// IsContainerdClient returns true if this is a containerd client
func (info *ClientInfo) IsContainerdClient() bool {
	return info.Name == "containerd"
}

// SupportsHTTP2 returns true if the client supports HTTP/2
func (info *ClientInfo) SupportsHTTP2() bool {
	for _, cap := range info.Capabilities {
		if cap == "http2" {
			return true
		}
	}
	return false
}

// HasCapability checks if client has a specific capability
func (info *ClientInfo) HasCapability(capability string) bool {
	for _, cap := range info.Capabilities {
		if cap == capability {
			return true
		}
	}
	return false
}

// AddWorkaround records that a workaround is active for this client
func (info *ClientInfo) AddWorkaround(name string) {
	info.Workarounds = append(info.Workarounds, name)
}

// String returns a human-readable representation
func (info *ClientInfo) String() string {
	if info.Version != "" {
		return info.Name + "/" + info.Version
	}
	return info.Name
}
