package automation

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// execTimeout is the maximum wall-clock time a Starlark script may run.
	execTimeout = 30 * time.Second

	// httpTimeout is the per-request timeout for outbound HTTP calls made by scripts.
	httpTimeout = 10 * time.Second

	// maxResponseBytes caps how much of a response body is read back to the script.
	maxResponseBytes = 1 << 20 // 1 MiB
)

// privateIPBlocks lists all RFC-private, loopback, and link-local CIDR ranges.
var privateIPBlocks []*net.IPNet

func init() {
	private := []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC-1918
		"172.16.0.0/12",  // RFC-1918
		"192.168.0.0/16", // RFC-1918
		"169.254.0.0/16", // Link-local (AWS metadata, etc.)
		"100.64.0.0/10",  // Shared address space (RFC-6598)
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique-local
		"fe80::/10",      // IPv6 link-local
	}
	for _, cidr := range private {
		_, block, _ := net.ParseCIDR(cidr)
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

// isPrivateIP returns true if ip falls in any private/reserved range.
func isPrivateIP(ip net.IP) bool {
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// validateURL rejects URLs that point at private/loopback addresses, blocking
// SSRF attacks from Starlark scripts.
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https URLs are allowed, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL has no host")
	}
	// Reject bare IPs that are private.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("requests to private IP addresses are not allowed")
		}
		return nil
	}
	// Resolve hostname and reject if any resolved IP is private.
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("DNS lookup failed for %q: %w", host, err)
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("requests to private IP addresses are not allowed (DNS resolved to %s)", addr)
		}
	}
	return nil
}

// safeDial wraps net.Dial with a post-connect IP check to guard against DNS
// rebinding attacks: the attacker first returns a public IP (which passes
// validateURL), then serves a private IP on subsequent lookups.
func safeDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, raw := range ips {
		ip := net.ParseIP(raw)
		if ip != nil && isPrivateIP(ip) {
			return nil, fmt.Errorf("connection to private IP %s blocked", raw)
		}
	}
	dialer := &net.Dialer{}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
}

// sandboxedClient is a pre-built HTTP client that:
//   - Refuses connections to private/loopback IPs (SSRF protection)
//   - Enforces a 10-second request timeout
//   - Does not follow redirects to private addresses
var sandboxedClient = &http.Client{
	Timeout: httpTimeout,
	Transport: &http.Transport{
		DialContext: safeDial,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if err := validateURL(req.URL.String()); err != nil {
			return fmt.Errorf("redirect blocked: %w", err)
		}
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		return nil
	},
}

// sandboxedPost performs an HTTP POST with SSRF protection and response-size
// capping. It is the implementation backing the http_post Starlark builtin.
func sandboxedPost(rawURL, body string) (int, string, error) {
	if err := validateURL(rawURL); err != nil {
		return 0, "", err
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", rawURL, nil)
	if err != nil {
		return 0, "", fmt.Errorf("http request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if body != "" {
		req.Body = io.NopCloser(io.LimitReader(strings.NewReader(body), maxResponseBytes))
		req.ContentLength = int64(len(body))
	}

	resp, err := sandboxedClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("http post failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	return resp.StatusCode, string(respBody), nil
}
