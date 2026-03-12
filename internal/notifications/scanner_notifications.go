package notifications

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/ryan/ads-registry/internal/scanner"
)

// ScanNotificationService handles notifications for security scan results
type ScanNotificationService struct {
	db              *sql.DB
	emailService    EmailService
	webhookService  WebhookService
	slackService    SlackService
}

// NewScanNotificationService creates a new scan notification service
func NewScanNotificationService(db *sql.DB) *ScanNotificationService {
	return &ScanNotificationService{
		db:             db,
		emailService:   NewEmailService(),
		webhookService: NewWebhookService(),
		slackService:   NewSlackService(),
	}
}

// NotifyOwnerOfScanResults notifies image owners of scan results
func (s *ScanNotificationService) NotifyOwnerOfScanResults(ctx context.Context, digest string, report *scanner.Report) error {
	// 1. Get the image owner
	ownerID, ownerEmail, err := s.getImageOwner(ctx, digest)
	if err != nil {
		log.Printf("[Notifications] Failed to get owner for %s: %v", digest, err)
		return err
	}

	if ownerID == 0 {
		log.Printf("[Notifications] No owner found for %s, skipping notification", digest)
		return nil
	}

	// 2. Get owner's notification preferences
	prefs, err := s.getNotificationPreferences(ctx, ownerID)
	if err != nil {
		log.Printf("[Notifications] Failed to get preferences for owner %d: %v", ownerID, err)
		// Continue with default preferences
		prefs = &NotificationPreferences{
			UserID:              ownerID,
			EmailEnabled:        true,
			CVEThreshold:        "HIGH",
			ImmediateNotification: true,
		}
	}

	// 3. Analyze scan results
	analysis := s.analyzeScanResults(report)

	// 4. Check if notification should be sent based on severity threshold
	if !s.shouldNotify(analysis, prefs) {
		log.Printf("[Notifications] Scan results for %s don't meet threshold, skipping notification", digest)
		return nil
	}

	// 5. Send notifications via configured channels
	if prefs.EmailEnabled {
		if err := s.sendEmailNotification(ctx, ownerEmail, digest, analysis); err != nil {
			log.Printf("[Notifications] Failed to send email to %s: %v", ownerEmail, err)
		}
	}

	if prefs.WebhookEnabled && prefs.WebhookURL != "" {
		if err := s.sendWebhookNotification(ctx, prefs.WebhookURL, digest, analysis); err != nil {
			log.Printf("[Notifications] Failed to send webhook to %s: %v", prefs.WebhookURL, err)
		}
	}

	if prefs.SlackEnabled && prefs.SlackWebhook != "" {
		if err := s.sendSlackNotification(ctx, prefs.SlackWebhook, digest, analysis); err != nil {
			log.Printf("[Notifications] Failed to send Slack notification: %v", err)
		}
	}

	// 6. Record notification in audit log
	if err := s.recordNotificationSent(ctx, ownerID, digest, analysis.CriticalCount+analysis.HighCount); err != nil {
		log.Printf("[Notifications] Failed to record notification: %v", err)
	}

	return nil
}

// ScanAnalysis contains analysis of scan results
type ScanAnalysis struct {
	TotalVulns    int
	CriticalCount int
	HighCount     int
	MediumCount   int
	LowCount      int
	HighestSeverity string
	TopCVEs       []scanner.Vuln
}

// NotificationPreferences represents user's notification settings
type NotificationPreferences struct {
	UserID                int
	EmailEnabled          bool
	WebhookEnabled        bool
	WebhookURL            string
	SlackEnabled          bool
	SlackWebhook          string
	CVEThreshold          string // CRITICAL, HIGH, MEDIUM, LOW
	ImmediateNotification bool
	DailyDigest           bool
	WeeklyDigest          bool
}

// getImageOwner retrieves the owner of an image by digest
func (s *ScanNotificationService) getImageOwner(ctx context.Context, digest string) (int, string, error) {
	query := `
		SELECT r.owner_id, u.email
		FROM manifests m
		JOIN repositories r ON m.repository_id = r.id
		LEFT JOIN users u ON r.owner_id = u.id
		WHERE m.digest = $1
		LIMIT 1
	`

	var ownerID sql.NullInt64
	var email sql.NullString

	err := s.db.QueryRowContext(ctx, query, digest).Scan(&ownerID, &email)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, "", nil // No owner found
		}
		return 0, "", fmt.Errorf("failed to query owner: %w", err)
	}

	if !ownerID.Valid {
		return 0, "", nil
	}

	return int(ownerID.Int64), email.String, nil
}

// getNotificationPreferences gets user's notification preferences
func (s *ScanNotificationService) getNotificationPreferences(ctx context.Context, userID int) (*NotificationPreferences, error) {
	query := `
		SELECT email_enabled, webhook_enabled, COALESCE(webhook_url, ''),
		       slack_enabled, COALESCE(slack_webhook, ''),
		       COALESCE(cve_threshold, 'HIGH'),
		       COALESCE(immediate_notification, true),
		       COALESCE(daily_digest, false),
		       COALESCE(weekly_digest, false)
		FROM security_notification_preferences
		WHERE user_id = $1
	`

	prefs := &NotificationPreferences{UserID: userID}

	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&prefs.EmailEnabled,
		&prefs.WebhookEnabled,
		&prefs.WebhookURL,
		&prefs.SlackEnabled,
		&prefs.SlackWebhook,
		&prefs.CVEThreshold,
		&prefs.ImmediateNotification,
		&prefs.DailyDigest,
		&prefs.WeeklyDigest,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no preferences found for user %d", userID)
		}
		return nil, fmt.Errorf("failed to query preferences: %w", err)
	}

	return prefs, nil
}

// analyzeScanResults analyzes scan results for notification
func (s *ScanNotificationService) analyzeScanResults(report *scanner.Report) *ScanAnalysis {
	analysis := &ScanAnalysis{
		TotalVulns: len(report.Vulnerabilities),
		TopCVEs:    make([]scanner.Vuln, 0),
	}

	severityOrder := map[string]int{
		"CRITICAL": 4,
		"HIGH":     3,
		"MEDIUM":   2,
		"LOW":      1,
		"UNKNOWN":  0,
	}

	highestSeverityValue := 0

	for _, vuln := range report.Vulnerabilities {
		switch vuln.Severity {
		case "CRITICAL":
			analysis.CriticalCount++
		case "HIGH":
			analysis.HighCount++
		case "MEDIUM":
			analysis.MediumCount++
		case "LOW":
			analysis.LowCount++
		}

		// Track highest severity
		if severityOrder[vuln.Severity] > highestSeverityValue {
			highestSeverityValue = severityOrder[vuln.Severity]
			analysis.HighestSeverity = vuln.Severity
		}

		// Collect top critical/high CVEs (up to 10)
		if (vuln.Severity == "CRITICAL" || vuln.Severity == "HIGH") && len(analysis.TopCVEs) < 10 {
			analysis.TopCVEs = append(analysis.TopCVEs, vuln)
		}
	}

	return analysis
}

// shouldNotify determines if notification should be sent based on threshold
func (s *ScanNotificationService) shouldNotify(analysis *ScanAnalysis, prefs *NotificationPreferences) bool {
	if !prefs.ImmediateNotification {
		return false
	}

	switch prefs.CVEThreshold {
	case "CRITICAL":
		return analysis.CriticalCount > 0
	case "HIGH":
		return analysis.CriticalCount > 0 || analysis.HighCount > 0
	case "MEDIUM":
		return analysis.CriticalCount > 0 || analysis.HighCount > 0 || analysis.MediumCount > 0
	case "LOW":
		return analysis.TotalVulns > 0
	default:
		return analysis.CriticalCount > 0 || analysis.HighCount > 0
	}
}

// sendEmailNotification sends email notification to owner
func (s *ScanNotificationService) sendEmailNotification(ctx context.Context, email, digest string, analysis *ScanAnalysis) error {
	subject := fmt.Sprintf("[Security Alert] %d vulnerabilities found in image %s", analysis.TotalVulns, digest[:12])

	body := fmt.Sprintf(`
Security Scan Results

Image Digest: %s
Scan Time: %s

Vulnerability Summary:
  CRITICAL: %d
  HIGH:     %d
  MEDIUM:   %d
  LOW:      %d

`, digest, time.Now().Format(time.RFC3339), analysis.CriticalCount, analysis.HighCount, analysis.MediumCount, analysis.LowCount)

	if len(analysis.TopCVEs) > 0 {
		body += "Top Vulnerabilities:\n\n"
		for i, cve := range analysis.TopCVEs {
			body += fmt.Sprintf("%d. %s [%s] in %s %s\n", i+1, cve.ID, cve.Severity, cve.Package, cve.Version)
			if cve.FixVersion != "" {
				body += fmt.Sprintf("   Fixed in: %s\n", cve.FixVersion)
			}
		}
	}

	body += "\n---\nADS Container Registry Security Notifications"

	return s.emailService.Send(email, subject, body)
}

// sendWebhookNotification sends webhook notification
func (s *ScanNotificationService) sendWebhookNotification(ctx context.Context, webhookURL, digest string, analysis *ScanAnalysis) error {
	payload := map[string]interface{}{
		"event":       "image.scan.completed",
		"digest":      digest,
		"timestamp":   time.Now(),
		"total_vulns": analysis.TotalVulns,
		"critical":    analysis.CriticalCount,
		"high":        analysis.HighCount,
		"medium":      analysis.MediumCount,
		"low":         analysis.LowCount,
		"top_cves":    analysis.TopCVEs,
	}

	return s.webhookService.Send(webhookURL, payload)
}

// sendSlackNotification sends Slack notification
func (s *ScanNotificationService) sendSlackNotification(ctx context.Context, slackWebhook, digest string, analysis *ScanAnalysis) error {
	color := "good"
	if analysis.CriticalCount > 0 {
		color = "danger"
	} else if analysis.HighCount > 0 {
		color = "warning"
	}

	message := SlackMessage{
		Text: "Security Scan Completed",
		Attachments: []SlackAttachment{
			{
				Color: color,
				Title: fmt.Sprintf("Image: %s", digest[:12]),
				Fields: []SlackField{
					{Title: "Total Vulnerabilities", Value: fmt.Sprintf("%d", analysis.TotalVulns), Short: true},
					{Title: "Critical", Value: fmt.Sprintf("%d", analysis.CriticalCount), Short: true},
					{Title: "High", Value: fmt.Sprintf("%d", analysis.HighCount), Short: true},
					{Title: "Medium", Value: fmt.Sprintf("%d", analysis.MediumCount), Short: true},
				},
			},
		},
	}

	return s.slackService.Send(slackWebhook, message)
}

// recordNotificationSent records that notification was sent
func (s *ScanNotificationService) recordNotificationSent(ctx context.Context, userID int, digest string, vulnCount int) error {
	query := `
		INSERT INTO security_audit_log (user_id, action, target_type, target_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	metadata := map[string]interface{}{
		"notification_type": "scan_results",
		"vulnerability_count": vulnCount,
	}
	metadataJSON, _ := (&scanner.Report{}).MarshalJSON() // placeholder

	_, err := s.db.ExecContext(ctx, query, userID, "notification_sent", "scan", digest, metadataJSON, time.Now())
	return err
}

// SaveScanResultsToDatabase saves detailed scan results to the security tables
func (s *ScanNotificationService) SaveScanResultsToDatabase(ctx context.Context, manifestID int, report *scanner.Report) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get or create scanner record
	scannerID, err := s.getOrCreateScanner(ctx, tx, report.ScannerName)
	if err != nil {
		return fmt.Errorf("failed to get scanner: %w", err)
	}

	// Create scan job record
	jobID, err := s.createScanJob(ctx, tx, manifestID, scannerID)
	if err != nil {
		return fmt.Errorf("failed to create scan job: %w", err)
	}

	// Save vulnerability findings
	for _, vuln := range report.Vulnerabilities {
		if err := s.saveVulnerabilityFinding(ctx, tx, jobID, manifestID, vuln); err != nil {
			log.Printf("[Notifications] Failed to save vulnerability %s: %v", vuln.ID, err)
			// Continue with other vulnerabilities
		}
	}

	// Mark scan job as completed
	if err := s.markScanJobCompleted(ctx, tx, jobID); err != nil {
		return fmt.Errorf("failed to mark job completed: %w", err)
	}

	return tx.Commit()
}

func (s *ScanNotificationService) getOrCreateScanner(ctx context.Context, tx *sql.Tx, name string) (int, error) {
	var id int
	query := `
		INSERT INTO security_scanners (name, scanner_type, enabled)
		VALUES ($1, 'cve', true)
		ON CONFLICT (name) DO UPDATE SET updated_at = NOW()
		RETURNING id
	`
	err := tx.QueryRowContext(ctx, query, name).Scan(&id)
	return id, err
}

func (s *ScanNotificationService) createScanJob(ctx context.Context, tx *sql.Tx, manifestID, scannerID int) (int, error) {
	var jobID int
	query := `
		INSERT INTO security_scan_jobs (manifest_id, scanner_id, status, started_at)
		VALUES ($1, $2, 'running', NOW())
		RETURNING id
	`
	err := tx.QueryRowContext(ctx, query, manifestID, scannerID).Scan(&jobID)
	return jobID, err
}

func (s *ScanNotificationService) saveVulnerabilityFinding(ctx context.Context, tx *sql.Tx, jobID, manifestID int, vuln scanner.Vuln) error {
	query := `
		INSERT INTO vulnerability_findings (
			scan_job_id, manifest_id, cve_id, package_name, package_version,
			fixed_version, severity, description, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'open')
	`

	_, err := tx.ExecContext(ctx, query,
		jobID, manifestID, vuln.ID, vuln.Package, vuln.Version,
		vuln.FixVersion, vuln.Severity, vuln.Description)

	return err
}

func (s *ScanNotificationService) markScanJobCompleted(ctx context.Context, tx *sql.Tx, jobID int) error {
	query := `
		UPDATE security_scan_jobs
		SET status = 'completed', completed_at = NOW(),
		    duration_ms = EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000
		WHERE id = $1
	`
	_, err := tx.ExecContext(ctx, query, jobID)
	return err
}
