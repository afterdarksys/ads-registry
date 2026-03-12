package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"time"
)

// ============================================================================
// EMAIL SERVICE
// ============================================================================

// EmailService handles email notifications
type EmailService struct {
	smtpHost     string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	fromAddress  string
}

// NewEmailService creates a new email service
func NewEmailService() EmailService {
	return EmailService{
		smtpHost:     getEnv("SMTP_HOST", "localhost"),
		smtpPort:     getEnv("SMTP_PORT", "587"),
		smtpUsername: getEnv("SMTP_USERNAME", ""),
		smtpPassword: getEnv("SMTP_PASSWORD", ""),
		fromAddress:  getEnv("SMTP_FROM", "registry@afterdarksys.com"),
	}
}

// Send sends an email
func (e EmailService) Send(to, subject, body string) error {
	if e.smtpHost == "" || e.smtpHost == "localhost" {
		// Log instead of actually sending in development
		fmt.Printf("[Email] Would send to %s: %s\n%s\n", to, subject, body)
		return nil
	}

	auth := smtp.PlainAuth("", e.smtpUsername, e.smtpPassword, e.smtpHost)

	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s\r\n"+
		"Subject: %s\r\n"+
		"\r\n"+
		"%s\r\n", to, e.fromAddress, subject, body))

	addr := fmt.Sprintf("%s:%s", e.smtpHost, e.smtpPort)
	return smtp.SendMail(addr, auth, e.fromAddress, []string{to}, msg)
}

// ============================================================================
// WEBHOOK SERVICE
// ============================================================================

// WebhookService handles webhook notifications
type WebhookService struct {
	client *http.Client
}

// NewWebhookService creates a new webhook service
func NewWebhookService() WebhookService {
	return WebhookService{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send sends a webhook notification
func (w WebhookService) Send(url string, payload map[string]interface{}) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ADS-Registry-Scanner/1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}

// ============================================================================
// SLACK SERVICE
// ============================================================================

// SlackService handles Slack notifications
type SlackService struct {
	client *http.Client
}

// NewSlackService creates a new Slack service
func NewSlackService() SlackService {
	return SlackService{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SlackMessage represents a Slack message
type SlackMessage struct {
	Text        string             `json:"text"`
	Attachments []SlackAttachment  `json:"attachments,omitempty"`
}

// SlackAttachment represents a Slack message attachment
type SlackAttachment struct {
	Color  string       `json:"color,omitempty"`
	Title  string       `json:"title,omitempty"`
	Text   string       `json:"text,omitempty"`
	Fields []SlackField `json:"fields,omitempty"`
}

// SlackField represents a field in a Slack attachment
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// Send sends a Slack notification
func (s SlackService) Send(webhookURL string, message SlackMessage) error {
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}

// ============================================================================
// UTILITIES
// ============================================================================

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
