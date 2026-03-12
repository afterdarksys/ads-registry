package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ryan/ads-registry/internal/upstreams"
)

// UpstreamProxy handles proxying requests to upstream registries
type UpstreamProxy struct {
	manager *upstreams.Manager
	client  *http.Client
}

// NewUpstreamProxy creates a new upstream proxy
func NewUpstreamProxy(manager *upstreams.Manager) *UpstreamProxy {
	return &UpstreamProxy{
		manager: manager,
		client: &http.Client{
			Timeout: 5 * time.Minute,
			// Don't follow redirects - let the Docker client handle them
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// IsUpstream checks if the namespace is an upstream registry
func (p *UpstreamProxy) IsUpstream(ctx context.Context, namespace string) bool {
	if p.manager == nil {
		return false
	}

	upstreams, err := p.manager.ListUpstreams(ctx)
	if err != nil {
		return false
	}

	for _, upstream := range upstreams {
		if upstream.Name == namespace {
			return true
		}
	}
	return false
}

// ProxyManifest proxies a manifest GET/HEAD request to an upstream registry
func (p *UpstreamProxy) ProxyManifest(ctx context.Context, upstreamName, repository, reference string, method string) (*http.Response, error) {
	// Get upstream configuration
	upstream, err := p.manager.GetUpstreamToken(ctx, upstreamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream token: %w", err)
	}

	// Get full upstream info to determine endpoint
	upstreamInfo, err := p.manager.ListUpstreams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list upstreams: %w", err)
	}

	var targetUpstream *upstreams.UpstreamRegistry
	for _, u := range upstreamInfo {
		if u.Name == upstreamName {
			targetUpstream = u
			break
		}
	}

	if targetUpstream == nil {
		return nil, fmt.Errorf("upstream %s not found", upstreamName)
	}

	// Build upstream URL
	// Format: https://{endpoint}/v2/{repository}/manifests/{reference}
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", targetUpstream.Endpoint, repository, reference)

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication based on upstream type
	if err := p.addAuth(req, targetUpstream, upstream); err != nil {
		return nil, fmt.Errorf("failed to add auth: %w", err)
	}

	// Add Docker client headers
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.list.v2+json")
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")

	// Make request
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}

	return resp, nil
}

// ProxyBlob proxies a blob GET/HEAD request to an upstream registry
func (p *UpstreamProxy) ProxyBlob(ctx context.Context, upstreamName, repository, digest string, method string) (*http.Response, error) {
	// Get upstream configuration
	token, err := p.manager.GetUpstreamToken(ctx, upstreamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream token: %w", err)
	}

	// Get full upstream info
	upstreamInfo, err := p.manager.ListUpstreams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list upstreams: %w", err)
	}

	var targetUpstream *upstreams.UpstreamRegistry
	for _, u := range upstreamInfo {
		if u.Name == upstreamName {
			targetUpstream = u
			break
		}
	}

	if targetUpstream == nil {
		return nil, fmt.Errorf("upstream %s not found", upstreamName)
	}

	// Build upstream URL
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", targetUpstream.Endpoint, repository, digest)

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication
	if err := p.addAuth(req, targetUpstream, token); err != nil {
		return nil, fmt.Errorf("failed to add auth: %w", err)
	}

	// Make request
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}

	return resp, nil
}

// ProxyTagsList proxies a tags list request to an upstream registry
func (p *UpstreamProxy) ProxyTagsList(ctx context.Context, upstreamName, repository string) (*http.Response, error) {
	// Get upstream configuration
	token, err := p.manager.GetUpstreamToken(ctx, upstreamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream token: %w", err)
	}

	// Get full upstream info
	upstreamInfo, err := p.manager.ListUpstreams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list upstreams: %w", err)
	}

	var targetUpstream *upstreams.UpstreamRegistry
	for _, u := range upstreamInfo {
		if u.Name == upstreamName {
			targetUpstream = u
			break
		}
	}

	if targetUpstream == nil {
		return nil, fmt.Errorf("upstream %s not found", upstreamName)
	}

	// Build upstream URL
	url := fmt.Sprintf("https://%s/v2/%s/tags/list", targetUpstream.Endpoint, repository)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication
	if err := p.addAuth(req, targetUpstream, token); err != nil {
		return nil, fmt.Errorf("failed to add auth: %w", err)
	}

	// Make request
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}

	return resp, nil
}

// addAuth adds appropriate authentication headers based on upstream type
func (p *UpstreamProxy) addAuth(req *http.Request, upstream *upstreams.UpstreamRegistry, token string) error {
	switch upstream.Type {
	case upstreams.UpstreamTypeAWS:
		// AWS ECR uses Basic auth with AWS:token
		auth := base64.StdEncoding.EncodeToString([]byte("AWS:" + token))
		req.Header.Set("Authorization", "Basic "+auth)

	case upstreams.UpstreamTypeOracle:
		// Oracle OCI uses Basic auth with username:token
		// Username is stored in AccessKeyID (user OCID)
		auth := base64.StdEncoding.EncodeToString([]byte(upstream.AccessKeyID + ":" + token))
		req.Header.Set("Authorization", "Basic "+auth)

	case upstreams.UpstreamTypeDocker:
		// Docker Hub uses Bearer token
		req.Header.Set("Authorization", "Bearer "+token)

	case upstreams.UpstreamTypeGCP:
		// GCP uses oauth2accesstoken:token for Basic auth
		auth := base64.StdEncoding.EncodeToString([]byte("oauth2accesstoken:" + token))
		req.Header.Set("Authorization", "Basic "+auth)

	default:
		return fmt.Errorf("unsupported upstream type: %s", upstream.Type)
	}

	return nil
}

// CopyResponseHeaders copies relevant headers from upstream response to client response
func CopyResponseHeaders(dst http.ResponseWriter, src *http.Response) {
	// Copy important Docker registry headers
	headers := []string{
		"Content-Type",
		"Content-Length",
		"Docker-Content-Digest",
		"Docker-Distribution-API-Version",
		"Etag",
		"Location",
		"Range",
		"Docker-Upload-UUID",
	}

	for _, header := range headers {
		if value := src.Header.Get(header); value != "" {
			dst.Header().Set(header, value)
		}
	}

	// Copy Link header for pagination
	if link := src.Header.Get("Link"); link != "" {
		dst.Header().Set("Link", link)
	}
}

// WriteProxyResponse writes the upstream response to the client
func WriteProxyResponse(w http.ResponseWriter, upstreamResp *http.Response) error {
	defer upstreamResp.Body.Close()

	// Copy headers
	CopyResponseHeaders(w, upstreamResp)

	// Write status code
	w.WriteHeader(upstreamResp.StatusCode)

	// Copy body
	if upstreamResp.Body != nil {
		_, err := io.Copy(w, upstreamResp.Body)
		return err
	}

	return nil
}

// RewriteUpstreamPath rewrites the upstream-style path to work with standard Docker clients
// Example: /v2/aws-ecr/myapp/manifests/latest -> repository="myapp"
func RewriteUpstreamPath(fullPath string) (repository string) {
	// Path format: /v2/{upstream}/{repo}/...
	// We want to extract {repo}
	parts := strings.Split(strings.TrimPrefix(fullPath, "/v2/"), "/")
	if len(parts) >= 2 {
		// Skip first part (upstream name) and take the rest until /manifests or /blobs
		repository = parts[1]
	}
	return repository
}
