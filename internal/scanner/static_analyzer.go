package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryan/ads-registry/internal/storage"
)

// StaticAnalyzer performs static analysis on container images
type StaticAnalyzer struct {
	semgrepPath  string
	rulesPath    string
	tempDir      string
	detectors    []SecretDetector
	storage      storage.Provider
}

// NewStaticAnalyzer creates a new static analyzer
func NewStaticAnalyzer(sp storage.Provider) *StaticAnalyzer {
	return &StaticAnalyzer{
		semgrepPath: findSemgrepBinary(),
		rulesPath:   "/etc/semgrep/rules",
		tempDir:     "/tmp/static-analysis",
		detectors:   initializeSecretDetectors(),
		storage:     sp,
	}
}

// Name returns the scanner name (implements Engine interface)
func (s *StaticAnalyzer) Name() string {
	return "semgrep-static-analysis"
}

// Scan implements the Engine interface for scanner integration
func (s *StaticAnalyzer) Scan(ctx context.Context, namespace, repo, digest string) (*Report, error) {
	log.Printf("[StaticAnalyzer] Scanning %s/%s@%s...", namespace, repo, digest)

	// 1. Extract image layers to temporary directory
	extractPath := filepath.Join(s.tempDir, digest)
	if err := os.MkdirAll(extractPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(extractPath) // Cleanup after scan

	// 2. Download and extract image layers
	if err := s.extractImageLayers(ctx, digest, extractPath); err != nil {
		return nil, fmt.Errorf("failed to extract image: %w", err)
	}

	// 3. Run static analysis
	staticReport, err := s.Analyze(ctx, digest, extractPath)
	if err != nil {
		return nil, fmt.Errorf("static analysis failed: %w", err)
	}

	// 4. Convert to standard scanner.Report format
	report := s.convertToScannerReport(staticReport)

	// 5. Save detailed static analysis report (JSON) for later retrieval
	detailedReportJSON, _ := json.Marshal(staticReport)
	reportPath := filepath.Join(s.tempDir, fmt.Sprintf("%s-static-analysis.json", digest))
	os.WriteFile(reportPath, detailedReportJSON, 0644)
	log.Printf("[StaticAnalyzer] Detailed report saved to %s", reportPath)

	return report, nil
}

// extractImageLayers extracts container image layers to a directory
func (s *StaticAnalyzer) extractImageLayers(ctx context.Context, digest, destDir string) error {
	log.Printf("[StaticAnalyzer] Extracting image layers for %s", digest)

	// Construct blob path (blobs/sha256/abc123...)
	blobPath := fmt.Sprintf("blobs/%s", digest)

	// Get blob size
	size, err := s.storage.Stat(ctx, blobPath)
	if err != nil {
		return fmt.Errorf("failed to stat blob: %w", err)
	}

	// Get blob from storage using Reader
	blobReader, err := s.storage.Reader(ctx, blobPath, 0)
	if err != nil {
		return fmt.Errorf("failed to get blob: %w", err)
	}
	defer blobReader.Close()

	log.Printf("[StaticAnalyzer] Downloaded blob %s (size: %d bytes)", digest, size)

	// For now, save the tarball and extract it
	tarPath := filepath.Join(destDir, "image.tar")
	tarFile, err := os.Create(tarPath)
	if err != nil {
		return fmt.Errorf("failed to create tar file: %w", err)
	}
	defer tarFile.Close()

	if _, err := io.Copy(tarFile, blobReader); err != nil {
		return fmt.Errorf("failed to write tar: %w", err)
	}

	// Extract tarball using tar command
	cmd := exec.CommandContext(ctx, "tar", "-xf", tarPath, "-C", destDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[StaticAnalyzer] Tar extraction output: %s", string(output))
		return fmt.Errorf("tar extraction failed: %w", err)
	}

	log.Printf("[StaticAnalyzer] Successfully extracted image to %s", destDir)
	return nil
}

// convertToScannerReport converts StaticAnalysisReport to standard scanner.Report
func (s *StaticAnalyzer) convertToScannerReport(staticReport *StaticAnalysisReport) *Report {
	var vulns []Vuln

	// Convert secrets to vulnerabilities
	for _, secret := range staticReport.Secrets {
		vulns = append(vulns, Vuln{
			ID:          fmt.Sprintf("SECRET-%s", strings.ToUpper(secret.Type)),
			Package:     secret.FilePath,
			Version:     "",
			FixVersion:  "Remove hardcoded secret",
			Severity:    strings.ToUpper(secret.Severity),
			Description: fmt.Sprintf("Hardcoded %s detected at line %d (confidence: %s)", secret.Type, secret.LineNumber, secret.Confidence),
		})
	}

	// Convert security findings to vulnerabilities
	for _, finding := range staticReport.Security {
		description := finding.Description
		if finding.CWE != "" {
			description = fmt.Sprintf("%s (%s)", description, finding.CWE)
		}

		vulns = append(vulns, Vuln{
			ID:          finding.RuleID,
			Package:     finding.FilePath,
			Version:     "",
			FixVersion:  finding.Fix,
			Severity:    strings.ToUpper(finding.Severity),
			Description: description,
		})
	}

	// Convert code smells to LOW severity vulnerabilities (informational)
	for _, smell := range staticReport.CodeSmells {
		vulns = append(vulns, Vuln{
			ID:          smell.RuleID,
			Package:     smell.FilePath,
			Version:     "",
			FixVersion:  smell.Suggestion,
			Severity:    "LOW",
			Description: fmt.Sprintf("[Code Quality] %s", smell.Message),
		})
	}

	return &Report{
		Digest:          staticReport.Digest,
		ScannerName:     "semgrep-static-analysis",
		ScannerVersion:  "1.0.0",
		CreatedAt:       staticReport.ScanTime,
		Vulnerabilities: vulns,
	}
}

// StaticAnalysisReport represents the results of static analysis
type StaticAnalysisReport struct {
	Digest      string                `json:"digest"`
	ScanTime    time.Time             `json:"scan_time"`
	Secrets     []SecretFinding       `json:"secrets"`
	CodeSmells  []CodeSmellFinding    `json:"code_smells"`
	Security    []SecurityFinding     `json:"security_findings"`
	Dockerfile  *DockerfileAnalysis   `json:"dockerfile,omitempty"`
	Summary     StaticAnalysisSummary `json:"summary"`
}

// SecretFinding represents a detected secret
type SecretFinding struct {
	Type        string  `json:"type"`         // api_key, password, private_key, aws_access_key, etc.
	FilePath    string  `json:"file_path"`
	LineNumber  int     `json:"line_number"`
	Snippet     string  `json:"snippet"`      // Redacted snippet
	Entropy     float64 `json:"entropy"`      // Shannon entropy score
	Confidence  string  `json:"confidence"`   // high, medium, low
	Severity    string  `json:"severity"`     // critical, high, medium
}

// CodeSmellFinding represents code quality issues
type CodeSmellFinding struct {
	RuleID      string `json:"rule_id"`
	Category    string `json:"category"`     // complexity, duplication, maintainability
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// SecurityFinding represents security vulnerabilities in code
type SecurityFinding struct {
	RuleID      string `json:"rule_id"`
	CWE         string `json:"cwe,omitempty"`      // CWE-89, CWE-79, etc.
	OWASP       string `json:"owasp,omitempty"`    // OWASP Top 10 category
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Fix         string `json:"fix,omitempty"`
}

// DockerfileAnalysis represents Dockerfile-specific findings
type DockerfileAnalysis struct {
	HasRootUser       bool     `json:"has_root_user"`
	HasSecretsInEnv   bool     `json:"has_secrets_in_env"`
	UsesLatestTag     bool     `json:"uses_latest_tag"`
	MissingHealthCheck bool    `json:"missing_health_check"`
	Issues            []string `json:"issues"`
	Score             int      `json:"score"` // 0-100
}

// StaticAnalysisSummary provides summary statistics
type StaticAnalysisSummary struct {
	TotalFindings      int `json:"total_findings"`
	CriticalSecrets    int `json:"critical_secrets"`
	HighSeverity       int `json:"high_severity"`
	MediumSeverity     int `json:"medium_severity"`
	LowSeverity        int `json:"low_severity"`
	FilesScanned       int `json:"files_scanned"`
	LinesOfCodeScanned int `json:"lines_of_code_scanned"`
}

// Analyze performs static analysis on an extracted container image
func (s *StaticAnalyzer) Analyze(ctx context.Context, imageDigest, extractPath string) (*StaticAnalysisReport, error) {
	log.Printf("[StaticAnalyzer] Starting analysis for %s", imageDigest)

	report := &StaticAnalysisReport{
		Digest:   imageDigest,
		ScanTime: time.Now(),
		Secrets:  []SecretFinding{},
		CodeSmells: []CodeSmellFinding{},
		Security: []SecurityFinding{},
	}

	// 1. Scan for secrets using pattern matching
	secrets, err := s.scanForSecrets(extractPath)
	if err != nil {
		log.Printf("[StaticAnalyzer] Secret scan failed: %v", err)
	} else {
		report.Secrets = secrets
		log.Printf("[StaticAnalyzer] Found %d potential secrets", len(secrets))
	}

	// 2. Run Semgrep for code smells and security issues
	if s.semgrepPath != "" {
		codeSmells, securityFindings, err := s.runSemgrep(ctx, extractPath)
		if err != nil {
			log.Printf("[StaticAnalyzer] Semgrep scan failed: %v", err)
		} else {
			report.CodeSmells = codeSmells
			report.Security = securityFindings
			log.Printf("[StaticAnalyzer] Semgrep found %d code smells, %d security issues",
				len(codeSmells), len(securityFindings))
		}
	}

	// 3. Analyze Dockerfile if present
	dockerfilePath := filepath.Join(extractPath, "Dockerfile")
	if fileExists(dockerfilePath) {
		dockerfileAnalysis := s.analyzeDockerfile(dockerfilePath)
		report.Dockerfile = dockerfileAnalysis
		log.Printf("[StaticAnalyzer] Dockerfile score: %d/100", dockerfileAnalysis.Score)
	}

	// 4. Calculate summary
	report.Summary = s.calculateSummary(report)

	log.Printf("[StaticAnalyzer] Analysis complete. Total findings: %d", report.Summary.TotalFindings)
	return report, nil
}

// scanForSecrets scans files for hardcoded secrets
func (s *StaticAnalyzer) scanForSecrets(rootPath string) ([]SecretFinding, error) {
	var findings []SecretFinding

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't read
		}

		// Skip binary files and common exclusions
		if info.IsDir() || info.Size() > 10*1024*1024 || isBinaryFile(path) {
			return nil
		}

		// Read file
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Scan with each detector
		for _, detector := range s.detectors {
			matches := detector.Detect(string(content))
			for _, match := range matches {
				finding := SecretFinding{
					Type:       detector.Name,
					FilePath:   strings.TrimPrefix(path, rootPath+"/"),
					LineNumber: match.LineNumber,
					Snippet:    redactSecret(match.Value),
					Entropy:    calculateEntropy(match.Value),
					Confidence: match.Confidence,
					Severity:   detector.Severity,
				}
				findings = append(findings, finding)
			}
		}

		return nil
	})

	return findings, err
}

// runSemgrep executes Semgrep to find code smells and security issues
func (s *StaticAnalyzer) runSemgrep(ctx context.Context, scanPath string) ([]CodeSmellFinding, []SecurityFinding, error) {
	if s.semgrepPath == "" {
		return nil, nil, fmt.Errorf("semgrep not found")
	}

	// Run Semgrep
	cmd := exec.CommandContext(ctx, s.semgrepPath,
		"--config", "auto",
		"--json",
		"--metrics", "off",
		scanPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Semgrep returns non-zero if findings exist, check output
		if len(output) == 0 {
			return nil, nil, fmt.Errorf("semgrep failed: %w", err)
		}
	}

	// Parse Semgrep JSON output
	var semgrepResults SemgrepOutput
	if err := json.Unmarshal(output, &semgrepResults); err != nil {
		return nil, nil, fmt.Errorf("failed to parse semgrep output: %w", err)
	}

	// Convert to our format
	var codeSmells []CodeSmellFinding
	var securityFindings []SecurityFinding

	for _, result := range semgrepResults.Results {
		if isSecurityRule(result.CheckID) {
			finding := SecurityFinding{
				RuleID:      result.CheckID,
				CWE:         extractCWE(result.Extra),
				OWASP:       extractOWASP(result.Extra),
				FilePath:    result.Path,
				LineNumber:  result.Start.Line,
				Description: result.Extra.Message,
				Severity:    result.Extra.Severity,
				Fix:         result.Extra.Fix,
			}
			securityFindings = append(securityFindings, finding)
		} else {
			smell := CodeSmellFinding{
				RuleID:     result.CheckID,
				Category:   categorizeCodeSmell(result.CheckID),
				FilePath:   result.Path,
				LineNumber: result.Start.Line,
				Message:    result.Extra.Message,
				Severity:   result.Extra.Severity,
				Suggestion: result.Extra.Fix,
			}
			codeSmells = append(codeSmells, smell)
		}
	}

	return codeSmells, securityFindings, nil
}

// analyzeDockerfile performs Dockerfile-specific analysis
func (s *StaticAnalyzer) analyzeDockerfile(dockerfilePath string) *DockerfileAnalysis {
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")
	analysis := &DockerfileAnalysis{
		Issues: []string{},
		Score:  100, // Start with perfect score
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for root user
		if strings.HasPrefix(line, "USER root") {
			analysis.HasRootUser = true
			analysis.Issues = append(analysis.Issues, "Running as root user")
			analysis.Score -= 20
		}

		// Check for secrets in ENV
		if strings.HasPrefix(line, "ENV") && containsSecretKeywords(line) {
			analysis.HasSecretsInEnv = true
			analysis.Issues = append(analysis.Issues, "Potential secret in ENV variable")
			analysis.Score -= 30
		}

		// Check for :latest tag
		if strings.Contains(line, ":latest") {
			analysis.UsesLatestTag = true
			analysis.Issues = append(analysis.Issues, "Using :latest tag (not reproducible)")
			analysis.Score -= 10
		}
	}

	// Check for HEALTHCHECK
	if !strings.Contains(string(content), "HEALTHCHECK") {
		analysis.MissingHealthCheck = true
		analysis.Issues = append(analysis.Issues, "Missing HEALTHCHECK instruction")
		analysis.Score -= 15
	}

	if analysis.Score < 0 {
		analysis.Score = 0
	}

	return analysis
}

// calculateSummary generates summary statistics
func (s *StaticAnalyzer) calculateSummary(report *StaticAnalysisReport) StaticAnalysisSummary {
	summary := StaticAnalysisSummary{}

	// Count secrets by severity
	for _, secret := range report.Secrets {
		summary.TotalFindings++
		if secret.Severity == "critical" {
			summary.CriticalSecrets++
		}
	}

	// Count code smells and security findings
	for _, smell := range report.CodeSmells {
		summary.TotalFindings++
		switch smell.Severity {
		case "high":
			summary.HighSeverity++
		case "medium":
			summary.MediumSeverity++
		case "low":
			summary.LowSeverity++
		}
	}

	for _, finding := range report.Security {
		summary.TotalFindings++
		switch finding.Severity {
		case "critical", "high":
			summary.HighSeverity++
		case "medium":
			summary.MediumSeverity++
		case "low":
			summary.LowSeverity++
		}
	}

	return summary
}

// Helper types and functions

type SemgrepOutput struct {
	Results []SemgrepResult `json:"results"`
}

type SemgrepResult struct {
	CheckID string             `json:"check_id"`
	Path    string             `json:"path"`
	Start   SemgrepLocation    `json:"start"`
	End     SemgrepLocation    `json:"end"`
	Extra   SemgrepExtra       `json:"extra"`
}

type SemgrepLocation struct {
	Line   int `json:"line"`
	Column int `json:"col"`
}

type SemgrepExtra struct {
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Fix      string `json:"fix,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

func findSemgrepBinary() string {
	paths := []string{
		"/usr/local/bin/semgrep",
		"/usr/bin/semgrep",
		"/opt/semgrep/bin/semgrep",
	}

	for _, path := range paths {
		if fileExists(path) {
			return path
		}
	}

	// Try PATH
	if path, err := exec.LookPath("semgrep"); err == nil {
		return path
	}

	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isBinaryFile(path string) bool {
	// Simple heuristic: check file extension
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := []string{".exe", ".dll", ".so", ".dylib", ".a", ".o", ".bin", ".jpg", ".png", ".gif", ".pdf", ".zip", ".tar", ".gz"}
	for _, bext := range binaryExts {
		if ext == bext {
			return true
		}
	}
	return false
}

func redactSecret(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "***" + value[len(value)-4:]
}

func calculateEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}

	freq := make(map[rune]float64)
	for _, c := range s {
		freq[c]++
	}

	var entropy float64
	length := float64(len(s))
	for _, count := range freq {
		p := count / length
		entropy -= p * (logBase2(p))
	}

	return entropy
}

func logBase2(x float64) float64 {
	if x == 0 {
		return 0
	}
	return float64(31 - bits.LeadingZeros32(uint32(x)))
}

func isSecurityRule(ruleID string) bool {
	securityKeywords := []string{"security", "injection", "xss", "sqli", "auth", "crypto", "cwe", "owasp"}
	lowerID := strings.ToLower(ruleID)
	for _, keyword := range securityKeywords {
		if strings.Contains(lowerID, keyword) {
			return true
		}
	}
	return false
}

func categorizeCodeSmell(ruleID string) string {
	if strings.Contains(ruleID, "complexity") {
		return "complexity"
	}
	if strings.Contains(ruleID, "duplicate") {
		return "duplication"
	}
	return "maintainability"
}

func extractCWE(extra SemgrepExtra) string {
	if extra.Metadata == nil {
		return ""
	}
	if cwe, ok := extra.Metadata["cwe"].(string); ok {
		return cwe
	}
	return ""
}

func extractOWASP(extra SemgrepExtra) string {
	if extra.Metadata == nil {
		return ""
	}
	if owasp, ok := extra.Metadata["owasp"].(string); ok {
		return owasp
	}
	return ""
}

func containsSecretKeywords(line string) bool {
	keywords := []string{"password", "secret", "token", "api_key", "private_key", "aws_access"}
	lower := strings.ToLower(line)
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
