package scanner

import (
	"regexp"
	"strings"
)

// SecretDetector detects secrets in code
type SecretDetector struct {
	Name     string
	Severity string
	Pattern  *regexp.Regexp
	Validate func(string) bool // Optional custom validation
}

// SecretMatch represents a detected secret
type SecretMatch struct {
	Value      string
	LineNumber int
	Confidence string // high, medium, low
}

// initializeSecretDetectors creates all secret detectors
func initializeSecretDetectors() []SecretDetector {
	return []SecretDetector{
		// AWS Access Keys
		{
			Name:     "aws_access_key",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(AKIA|A3T|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[0-9A-Z]{16}`),
			Validate: func(s string) bool {
				return len(s) == 20 && strings.HasPrefix(s, "AKIA")
			},
		},
		// AWS Secret Access Keys
		{
			Name:     "aws_secret_key",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)aws(.{0,20})?['\"][0-9a-zA-Z/+]{40}['\"]`),
		},
		// GitHub Personal Access Tokens
		{
			Name:     "github_token",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`ghp_[0-9a-zA-Z]{36}`),
		},
		// GitHub OAuth Tokens
		{
			Name:     "github_oauth",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`gho_[0-9a-zA-Z]{36}`),
		},
		// GitLab Personal Access Tokens
		{
			Name:     "gitlab_token",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`glpat-[0-9a-zA-Z\-\_]{20}`),
		},
		// Private Keys
		{
			Name:     "private_key",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`-----BEGIN (RSA|DSA|EC|OPENSSH) PRIVATE KEY-----`),
		},
		// Generic API Keys (high entropy)
		{
			Name:     "api_key",
			Severity: "high",
			Pattern:  regexp.MustCompile(`(?i)(api[_-]?key|apikey|api[_-]?secret)['\"]?\s*[:=]\s*['\"]?([0-9a-zA-Z\-_]{20,})['\"]?`),
			Validate: func(s string) bool {
				// Check if it has high entropy
				return calculateEntropy(s) > 4.0
			},
		},
		// Generic Passwords
		{
			Name:     "password",
			Severity: "high",
			Pattern:  regexp.MustCompile(`(?i)(password|passwd|pwd)['\"]?\s*[:=]\s*['\"]([^'\"\s]{8,})['\"]`),
		},
		// Database Connection Strings
		{
			Name:     "database_url",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)(postgresql|mysql|mongodb|redis)://[^:]+:[^@]+@[^/]+`),
		},
		// Slack Tokens
		{
			Name:     "slack_token",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`xox[baprs]-[0-9]{10,13}-[0-9]{10,13}-[a-zA-Z0-9]{24,32}`),
		},
		// Slack Webhooks
		{
			Name:     "slack_webhook",
			Severity: "high",
			Pattern:  regexp.MustCompile(`https://hooks\.slack\.com/services/T[a-zA-Z0-9_]+/B[a-zA-Z0-9_]+/[a-zA-Z0-9_]+`),
		},
		// Stripe API Keys
		{
			Name:     "stripe_key",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`sk_live_[0-9a-zA-Z]{24,}`),
		},
		// Google API Keys
		{
			Name:     "google_api_key",
			Severity: "high",
			Pattern:  regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
		},
		// Google OAuth Client Secret
		{
			Name:     "google_oauth",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`[0-9]+-[0-9A-Za-z_]{32}\.apps\.googleusercontent\.com`),
		},
		// Heroku API Keys
		{
			Name:     "heroku_key",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`[h|H][e|E][r|R][o|O][k|K][u|U].{0,30}[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}`),
		},
		// MailChimp API Keys
		{
			Name:     "mailchimp_key",
			Severity: "high",
			Pattern:  regexp.MustCompile(`[0-9a-f]{32}-us[0-9]{1,2}`),
		},
		// Mailgun API Keys
		{
			Name:     "mailgun_key",
			Severity: "high",
			Pattern:  regexp.MustCompile(`key-[0-9a-zA-Z]{32}`),
		},
		// PayPal Braintree Access Token
		{
			Name:     "paypal_token",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`access_token\$production\$[0-9a-z]{16}\$[0-9a-f]{32}`),
		},
		// Square Access Token
		{
			Name:     "square_token",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`sq0atp-[0-9A-Za-z\-_]{22}`),
		},
		// Twilio API Keys
		{
			Name:     "twilio_key",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`SK[0-9a-fA-F]{32}`),
		},
		// Twitter Access Token
		{
			Name:     "twitter_token",
			Severity: "high",
			Pattern:  regexp.MustCompile(`[t|T][w|W][i|I][t|T][t|T][e|E][r|R].{0,30}[1-9][0-9]+-[0-9a-zA-Z]{40}`),
		},
		// NPM Tokens
		{
			Name:     "npm_token",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`npm_[0-9a-zA-Z]{36}`),
		},
		// Docker Hub Tokens
		{
			Name:     "dockerhub_token",
			Severity: "high",
			Pattern:  regexp.MustCompile(`(?i)dockerhub.{0,30}['\"][0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}['\"]`),
		},
		// JWT Tokens
		{
			Name:     "jwt_token",
			Severity: "high",
			Pattern:  regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`),
			Validate: func(s string) bool {
				// Basic JWT structure validation
				parts := strings.Split(s, ".")
				return len(parts) == 3
			},
		},
		// Generic Secrets (high entropy strings)
		{
			Name:     "generic_secret",
			Severity: "medium",
			Pattern:  regexp.MustCompile(`(?i)(secret|token|key)['\"]?\s*[:=]\s*['\"]([0-9a-zA-Z\-_]{32,})['\"]`),
			Validate: func(s string) bool {
				return calculateEntropy(s) > 4.5
			},
		},
	}
}

// Detect scans content for secrets
func (d SecretDetector) Detect(content string) []SecretMatch {
	var matches []SecretMatch

	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		// Skip comments
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Find all matches on this line
		foundMatches := d.Pattern.FindAllString(line, -1)
		for _, match := range foundMatches {
			// Skip false positives
			if d.isFalsePositive(match) {
				continue
			}

			// Custom validation if provided
			if d.Validate != nil && !d.Validate(match) {
				continue
			}

			// Determine confidence based on entropy and pattern
			confidence := d.determineConfidence(match)

			matches = append(matches, SecretMatch{
				Value:      match,
				LineNumber: lineNum + 1,
				Confidence: confidence,
			})
		}
	}

	return matches
}

// isFalsePositive checks if a match is likely a false positive
func (d SecretDetector) isFalsePositive(match string) bool {
	falsePositives := []string{
		"example",
		"sample",
		"test",
		"dummy",
		"fake",
		"your_",
		"my_",
		"<your",
		"<insert",
		"TODO",
		"FIXME",
		"XXX",
		"YYY",
		"ZZZ",
		"0000000000",
		"1111111111",
		"xxxxxxxx",
		"XXXXXXXX",
	}

	lowerMatch := strings.ToLower(match)
	for _, fp := range falsePositives {
		if strings.Contains(lowerMatch, strings.ToLower(fp)) {
			return true
		}
	}

	return false
}

// determineConfidence calculates confidence level based on entropy and pattern
func (d SecretDetector) determineConfidence(match string) string {
	entropy := calculateEntropy(match)

	// High confidence: known pattern + high entropy
	if d.Name == "aws_access_key" || d.Name == "github_token" || d.Name == "private_key" {
		return "high"
	}

	// Medium confidence: good entropy
	if entropy > 4.5 {
		return "high"
	} else if entropy > 3.5 {
		return "medium"
	}

	// Low confidence
	return "low"
}
