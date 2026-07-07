package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	maxAttempts = 5
	baseDelay   = time.Second
)

type Dispatcher struct {
	endpoints []string
	client    *http.Client
}

type Payload struct {
	Event     string            `json:"event"`
	Timestamp time.Time         `json:"timestamp"`
	Data      map[string]string `json:"data"`
}

func NewDispatcher(endpoints []string) *Dispatcher {
	return &Dispatcher{
		endpoints: endpoints,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// isDisallowedWebhookHost returns true when rawURL resolves to a loopback,
// link-local, or private IP range that a webhook should never reach.
// On any parse error the function returns true (fail-closed).
func isDisallowedWebhookHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := u.Hostname()
	if host == "" {
		return true
	}

	// Resolve hostname to IP(s). Use the literal IP if already numeric.
	addrs := []net.IP{net.ParseIP(host)}
	if addrs[0] == nil {
		resolved, err := net.LookupHost(host)
		if err != nil {
			// Cannot resolve — fail closed.
			return true
		}
		addrs = make([]net.IP, 0, len(resolved))
		for _, a := range resolved {
			if ip := net.ParseIP(a); ip != nil {
				addrs = append(addrs, ip)
			}
		}
	}

	// Private/reserved ranges that webhook targets must not be.
	disallowed := []net.IPNet{
		// IPv4 loopback
		{IP: net.ParseIP("127.0.0.0"), Mask: net.CIDRMask(8, 32)},
		// Link-local
		{IP: net.ParseIP("169.254.0.0"), Mask: net.CIDRMask(16, 32)},
		// RFC-1918 private
		{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)},
		{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)},
		{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)},
	}
	// IPv6 loopback (::1) and unique-local (fc00::/7)
	_, ipv6Loopback, _ := net.ParseCIDR("::1/128")
	_, ipv6ULA, _ := net.ParseCIDR("fc00::/7")

	for _, ip := range addrs {
		if ipv6Loopback != nil && ipv6Loopback.Contains(ip) {
			return true
		}
		if ipv6ULA != nil && ipv6ULA.Contains(ip) {
			return true
		}
		for _, block := range disallowed {
			if block.Contains(ip) {
				return true
			}
		}
	}
	return false
}

func (d *Dispatcher) Dispatch(ctx context.Context, event string, data map[string]string) {
	if len(d.endpoints) == 0 {
		return
	}

	payload := Payload{
		Event:     event,
		Timestamp: time.Now(),
		Data:      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Webhook marshal error: %v", err)
		return
	}

	for _, endpoint := range d.endpoints {
		if isDisallowedWebhookHost(endpoint) {
			log.Printf("Webhook SSRF guard: skipping delivery to disallowed host %q", endpoint)
			continue
		}
		go d.deliverWithRetry(endpoint, body)
	}
}

// deliverWithRetry attempts to deliver body to url up to maxAttempts times
// using exponential backoff (1s, 2s, 4s, 8s, 16s). Failed deliveries after
// all attempts are logged as dead-letter entries.
func (d *Dispatcher) deliverWithRetry(url string, body []byte) {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := d.deliver(url, body)
		if err == nil {
			if attempt > 1 {
				log.Printf("Webhook delivered to %s after %d attempt(s)", url, attempt)
			}
			return
		}

		if attempt == maxAttempts {
			log.Printf("Webhook dead-letter: all %d attempts to %s failed: %v", maxAttempts, url, err)
			return
		}

		delay := baseDelay * (1 << (attempt - 1)) // 1s, 2s, 4s, 8s
		log.Printf("Webhook delivery to %s failed (attempt %d/%d), retrying in %s: %v", url, attempt, maxAttempts, delay, err)
		time.Sleep(delay)
	}
}

func (d *Dispatcher) deliver(url string, body []byte) error {
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
