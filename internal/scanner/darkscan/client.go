package darkscan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a client for the DarkScan vulnerability scanning API via darkapi.io
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new DarkScan API client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ScanRequest represents a request to scan a container image
type ScanRequest struct {
	Registry   string            `json:"registry"`   // e.g., "registry.afterdarksys.com"
	Repository string            `json:"repository"` // e.g., "library/nginx"
	Tag        string            `json:"tag"`        // e.g., "latest"
	Digest     string            `json:"digest"`     // e.g., "sha256:abc123..."
	MediaType  string            `json:"media_type"` // e.g., "application/vnd.docker.distribution.manifest.v2+json"
	Metadata   map[string]string `json:"metadata"`   // Optional metadata
}

// ScanResponse represents the response from initiating a scan
type ScanResponse struct {
	ScanID    string    `json:"scan_id"`
	Status    string    `json:"status"` // "queued", "scanning", "completed", "failed"
	Message   string    `json:"message"`
	QueuedAt  time.Time `json:"queued_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ScanResult represents the complete vulnerability scan results
type ScanResult struct {
	ScanID       string           `json:"scan_id"`
	Status       string           `json:"status"`
	CompletedAt  *time.Time       `json:"completed_at"`
	Registry     string           `json:"registry"`
	Repository   string           `json:"repository"`
	Tag          string           `json:"tag"`
	Digest       string           `json:"digest"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Summary      ScanSummary      `json:"summary"`
	MalwareFound bool             `json:"malware_found"`
	SecretsFound []Secret         `json:"secrets_found"`
}

// Vulnerability represents a single CVE finding
type Vulnerability struct {
	ID          string   `json:"id"`           // CVE ID, e.g., "CVE-2024-1234"
	Severity    string   `json:"severity"`     // "CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"
	Package     string   `json:"package"`      // Affected package name
	Version     string   `json:"version"`      // Affected package version
	FixedIn     string   `json:"fixed_in"`     // Version where it's fixed (if available)
	Title       string   `json:"title"`        // Vulnerability title
	Description string   `json:"description"`  // Detailed description
	Links       []string `json:"links"`        // Reference URLs
	CVSS        *CVSS    `json:"cvss"`         // CVSS score information
}

// CVSS represents Common Vulnerability Scoring System information
type CVSS struct {
	Score   float64 `json:"score"`   // 0.0 to 10.0
	Vector  string  `json:"vector"`  // CVSS vector string
	Version string  `json:"version"` // "3.1", "3.0", "2.0"
}

// ScanSummary provides a high-level overview of scan results
type ScanSummary struct {
	TotalVulnerabilities int `json:"total_vulnerabilities"`
	Critical             int `json:"critical"`
	High                 int `json:"high"`
	Medium               int `json:"medium"`
	Low                  int `json:"low"`
	Unknown              int `json:"unknown"`
	Fixable              int `json:"fixable"`
}

// Secret represents a detected secret/credential in the image
type Secret struct {
	Type        string `json:"type"`        // "api_key", "password", "private_key", etc.
	File        string `json:"file"`        // File path where found
	Line        int    `json:"line"`        // Line number
	Description string `json:"description"` // What was found
	Severity    string `json:"severity"`    // "HIGH", "MEDIUM", "LOW"
}

// SubmitScan submits a container image for vulnerability scanning
func (c *Client) SubmitScan(ctx context.Context, req *ScanRequest) (*ScanResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/scans", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to submit scan: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("scan submission failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var scanResp ScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&scanResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &scanResp, nil
}

// GetScanResult retrieves the results of a completed scan
func (c *Client) GetScanResult(ctx context.Context, scanID string) (*ScanResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/scans/"+scanID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get scan result: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get scan result failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result ScanResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetScanStatus checks the status of a scan without retrieving full results
func (c *Client) GetScanStatus(ctx context.Context, scanID string) (*ScanResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/scans/"+scanID+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get scan status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get scan status failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var status ScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &status, nil
}

// HealthCheck verifies the DarkScan API is reachable and credentials are valid
func (c *Client) HealthCheck(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}
