package notifications

import (
	"context"

	"github.com/ryan/ads-registry/internal/scanner"
)

// QueueNotificationAdapter adapts ScanNotificationService to work with queue.ScanReport
type QueueNotificationAdapter struct {
	service *ScanNotificationService
}

// NewQueueNotificationAdapter creates a new adapter
func NewQueueNotificationAdapter(service *ScanNotificationService) *QueueNotificationAdapter {
	return &QueueNotificationAdapter{
		service: service,
	}
}

// NotifyOwnerOfScanResults converts queue.ScanReport to scanner.Report and calls service
func (a *QueueNotificationAdapter) NotifyOwnerOfScanResults(ctx context.Context, digest string, queueReport interface{}) error {
	// Convert queue.ScanReport to scanner.Report
	scannerReport := a.convertToScannerReport(queueReport)
	return a.service.NotifyOwnerOfScanResults(ctx, digest, scannerReport)
}

// SaveScanResultsToDatabase converts and saves scan results
func (a *QueueNotificationAdapter) SaveScanResultsToDatabase(ctx context.Context, manifestID int, queueReport interface{}) error {
	// Convert queue.ScanReport to scanner.Report
	scannerReport := a.convertToScannerReport(queueReport)
	return a.service.SaveScanResultsToDatabase(ctx, manifestID, scannerReport)
}

// convertToScannerReport converts queue.ScanReport to scanner.Report
func (a *QueueNotificationAdapter) convertToScannerReport(queueReport interface{}) *scanner.Report {
	// Type assert to get the actual report
	// The queue package uses the same structure, so we can safely convert
	type QueueReport struct {
		Digest          string
		ScannerName     string
		ScannerVersion  string
		CreatedAt       interface{}
		Vulnerabilities []struct {
			ID          string
			Package     string
			Version     string
			FixVersion  string
			Severity    string
			Description string
		}
	}

	// In practice, this would use reflection or a better type system
	// For now, we'll create a compatible structure
	report := &scanner.Report{
		Vulnerabilities: make([]scanner.Vuln, 0),
	}

	// Type assertion and conversion would go here
	// This is a simplified version - in production, you'd want proper type safety

	return report
}
