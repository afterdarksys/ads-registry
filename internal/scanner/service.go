package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/scanner/darkscan"
)

// Service manages vulnerability scanning operations
type Service struct {
	db            db.Store
	darkscanClient *darkscan.Client
	config        config.DarkScanConfig
}

// NewService creates a new scanner service
func NewService(dbStore db.Store, cfg config.DarkScanConfig) *Service {
	var client *darkscan.Client
	if cfg.Enabled && cfg.APIKey != "" {
		client = darkscan.NewClient(cfg.BaseURL, cfg.APIKey)

		// Test connection on startup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.HealthCheck(ctx); err != nil {
			log.Printf("[SCANNER] Warning: DarkScan health check failed: %v", err)
			log.Printf("[SCANNER] Scans will fail until connection is established")
		} else {
			log.Printf("[SCANNER] DarkScan integration active (API: %s)", cfg.BaseURL)
		}
	}

	return &Service{
		db:            dbStore,
		darkscanClient: client,
		config:        cfg,
	}
}

// ScanImage submits an image for vulnerability scanning
func (s *Service) ScanImage(ctx context.Context, registry, repository, tag, digest, mediaType string) error {
	if !s.config.Enabled || s.darkscanClient == nil {
		log.Printf("[SCANNER] Scanning disabled or not configured")
		return nil
	}

	log.Printf("[SCANNER] Submitting scan for %s/%s:%s", repository, tag, digest[:12])

	req := &darkscan.ScanRequest{
		Registry:   registry,
		Repository: repository,
		Tag:        tag,
		Digest:     digest,
		MediaType:  mediaType,
		Metadata: map[string]string{
			"source": "ads-registry",
		},
	}

	// Submit scan
	resp, err := s.darkscanClient.SubmitScan(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to submit scan: %w", err)
	}

	log.Printf("[SCANNER] Scan queued: %s (status: %s)", resp.ScanID, resp.Status)

	// Store initial scan status
	initialReport := map[string]interface{}{
		"scan_id":    resp.ScanID,
		"status":     resp.Status,
		"queued_at":  resp.QueuedAt,
		"repository": repository,
		"tag":        tag,
		"digest":     digest,
	}

	reportJSON, _ := json.Marshal(initialReport)
	if err := s.db.SaveScanReport(ctx, digest, "darkscan", reportJSON); err != nil {
		log.Printf("[SCANNER] Warning: failed to save initial scan status: %v", err)
	}

	// If the scan completes quickly (synchronous mode), retrieve results immediately
	if resp.Status == "completed" {
		return s.retrieveAndStoreScanResults(ctx, resp.ScanID, digest)
	}

	// For async scans, results will be retrieved by a background worker
	return nil
}

// RetrieveScanResults fetches and stores the results of a completed scan
func (s *Service) RetrieveScanResults(ctx context.Context, scanID, digest string) error {
	if !s.config.Enabled || s.darkscanClient == nil {
		return fmt.Errorf("scanning not enabled")
	}

	return s.retrieveAndStoreScanResults(ctx, scanID, digest)
}

func (s *Service) retrieveAndStoreScanResults(ctx context.Context, scanID, digest string) error {
	result, err := s.darkscanClient.GetScanResult(ctx, scanID)
	if err != nil {
		return fmt.Errorf("failed to get scan result: %w", err)
	}

	log.Printf("[SCANNER] Scan %s completed: %d vulnerabilities found (%d critical, %d high)",
		scanID, result.Summary.TotalVulnerabilities, result.Summary.Critical, result.Summary.High)

	// Store full scan results
	reportJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal scan results: %w", err)
	}

	if err := s.db.SaveScanReport(ctx, digest, "darkscan", reportJSON); err != nil {
		return fmt.Errorf("failed to save scan report: %w", err)
	}

	return nil
}

// GetScanReport retrieves the stored scan report for an image
func (s *Service) GetScanReport(ctx context.Context, digest string) (*darkscan.ScanResult, error) {
	reportJSON, err := s.db.GetScanReport(ctx, digest, "darkscan")
	if err != nil {
		if err == db.ErrNotFound {
			return nil, nil // No scan report available
		}
		return nil, fmt.Errorf("failed to get scan report: %w", err)
	}

	var result darkscan.ScanResult
	if err := json.Unmarshal(reportJSON, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scan report: %w", err)
	}

	return &result, nil
}

// CheckPolicyViolations checks if an image violates security policies
func (s *Service) CheckPolicyViolations(ctx context.Context, digest string) (blocked bool, reason string, err error) {
	if !s.config.Enabled {
		return false, "", nil
	}

	report, err := s.GetScanReport(ctx, digest)
	if err != nil {
		return false, "", err
	}

	// No scan report yet - allow but queue scan
	if report == nil {
		return false, "", nil
	}

	// Check if scan is still in progress
	if report.Status != "completed" {
		return false, "", nil // Allow while scanning
	}

	// Block on critical vulnerabilities
	if s.config.BlockOnCritical && report.Summary.Critical > 0 {
		return true, fmt.Sprintf("Image has %d CRITICAL vulnerabilities", report.Summary.Critical), nil
	}

	// Block on high severity vulnerabilities
	if s.config.BlockOnHigh && report.Summary.High > 0 {
		return true, fmt.Sprintf("Image has %d HIGH severity vulnerabilities", report.Summary.High), nil
	}

	// Block if malware detected
	if report.MalwareFound {
		return true, "Malware detected in image", nil
	}

	return false, "", nil
}

// IsEnabled returns whether scanning is enabled
func (s *Service) IsEnabled() bool {
	return s.config.Enabled && s.darkscanClient != nil
}
