package clamav

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryan/ads-registry/internal/scanner"
	"github.com/ryan/ads-registry/internal/storage"
)

// Scanner wraps ClamAV for malware detection
type Scanner struct {
	clamdSocket string       // Unix socket or TCP address (e.g., /var/run/clamav/clamd.ctl or localhost:3310)
	useDaemon   bool         // Use clamd (daemon) vs clamscan (CLI)
	tempDir     string       // Temporary extraction directory
	storage     storage.Provider
}

// New creates a new ClamAV scanner
func New(clamdSocket string, sp storage.Provider) *Scanner {
	return &Scanner{
		clamdSocket: clamdSocket,
		useDaemon:   clamdSocket != "",
		tempDir:     "/tmp/clamav-scan",
		storage:     sp,
	}
}

// Name returns the scanner name
func (s *Scanner) Name() string {
	return "clamav-malware-scanner"
}

// Scan scans a container image for malware
func (s *Scanner) Scan(ctx context.Context, namespace, repo, digest string) (*scanner.Report, error) {
	log.Printf("[ClamAV] Scanning %s/%s@%s for malware...", namespace, repo, digest)

	// Create temp directory for this scan
	scanDir := filepath.Join(s.tempDir, digest)
	if err := os.MkdirAll(scanDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create scan directory: %w", err)
	}
	defer os.RemoveAll(scanDir)

	// Extract image layers
	if err := s.extractImageLayers(ctx, digest, scanDir); err != nil {
		return nil, fmt.Errorf("failed to extract image: %w", err)
	}

	// Scan for malware
	findings, err := s.scanDirectory(ctx, scanDir)
	if err != nil {
		return nil, fmt.Errorf("malware scan failed: %w", err)
	}

	// Convert findings to report
	report := &scanner.Report{
		Digest:          digest,
		ScannerName:     "clamav-malware-scanner",
		ScannerVersion:  s.getVersion(),
		CreatedAt:       time.Now(),
		Vulnerabilities: s.convertFindings(findings),
	}

	if len(findings) > 0 {
		log.Printf("[ClamAV] ⚠️  MALWARE DETECTED: Found %d threats in %s", len(findings), digest)
	} else {
		log.Printf("[ClamAV] ✓ Clean: No malware detected in %s", digest)
	}

	return report, nil
}

// MalwareFinding represents a detected malware
type MalwareFinding struct {
	FilePath    string
	ThreatName  string
	ThreatType  string // virus, trojan, backdoor, exploit, etc.
	Severity    string // critical, high, medium
}

// scanDirectory scans a directory for malware using ClamAV
func (s *Scanner) scanDirectory(ctx context.Context, scanPath string) ([]MalwareFinding, error) {
	if s.useDaemon {
		return s.scanWithClamd(ctx, scanPath)
	}
	return s.scanWithClamscan(ctx, scanPath)
}

// scanWithClamd scans using the ClamAV daemon (faster, recommended)
func (s *Scanner) scanWithClamd(ctx context.Context, scanPath string) ([]MalwareFinding, error) {
	log.Printf("[ClamAV] Scanning with clamd daemon at %s", s.clamdSocket)

	// Connect to clamd
	conn, err := s.connectToClamd()
	if err != nil {
		log.Printf("[ClamAV] Failed to connect to clamd: %v, falling back to clamscan", err)
		return s.scanWithClamscan(ctx, scanPath)
	}
	defer conn.Close()

	// Send CONTSCAN command (recursive scan with full path)
	command := fmt.Sprintf("CONTSCAN %s\n", scanPath)
	if _, err := conn.Write([]byte(command)); err != nil {
		return nil, fmt.Errorf("failed to send scan command: %w", err)
	}

	// Read response
	response := make([]byte, 64*1024) // 64KB buffer
	n, err := conn.Read(response)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read scan response: %w", err)
	}

	// Parse findings
	findings := s.parseClamscanOutput(string(response[:n]))
	return findings, nil
}

// scanWithClamscan scans using the clamscan CLI tool (slower, but doesn't require daemon)
func (s *Scanner) scanWithClamscan(ctx context.Context, scanPath string) ([]MalwareFinding, error) {
	log.Printf("[ClamAV] Scanning with clamscan CLI")

	// Check if clamscan is available
	clamscanPath, err := exec.LookPath("clamscan")
	if err != nil {
		return nil, fmt.Errorf("clamscan not found in PATH: %w", err)
	}

	// Run clamscan
	cmd := exec.CommandContext(ctx, clamscanPath,
		"--recursive",           // Scan subdirectories
		"--infected",            // Only print infected files
		"--no-summary",          // Don't print summary
		"--stdout",              // Output to stdout
		scanPath,
	)

	output, err := cmd.CombinedOutput()

	// clamscan returns exit code 1 if infected files found, 0 if clean, 2 on error
	if cmd.ProcessState.ExitCode() == 2 {
		return nil, fmt.Errorf("clamscan error: %s", string(output))
	}

	// Parse findings
	findings := s.parseClamscanOutput(string(output))
	return findings, nil
}

// connectToClamd connects to the ClamAV daemon
func (s *Scanner) connectToClamd() (net.Conn, error) {
	// Try unix socket first
	if strings.HasPrefix(s.clamdSocket, "/") {
		return net.Dial("unix", s.clamdSocket)
	}

	// Try TCP
	return net.Dial("tcp", s.clamdSocket)
}

// parseClamscanOutput parses clamscan/clamd output and extracts findings
func (s *Scanner) parseClamscanOutput(output string) []MalwareFinding {
	var findings []MalwareFinding

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Example output:
		// /tmp/scan/malware.exe: Win.Trojan.Agent-12345 FOUND
		// /tmp/scan/backdoor.sh: Unix.Backdoor.Mirai FOUND

		if strings.Contains(line, " FOUND") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}

			filePath := strings.TrimSpace(parts[0])
			threatInfo := strings.TrimSpace(parts[1])
			threatInfo = strings.TrimSuffix(threatInfo, " FOUND")
			threatName := strings.TrimSpace(threatInfo)

			finding := MalwareFinding{
				FilePath:   filePath,
				ThreatName: threatName,
				ThreatType: s.categorizeThreat(threatName),
				Severity:   s.determineSeverity(threatName),
			}

			findings = append(findings, finding)
		}
	}

	return findings
}

// categorizeThreat categorizes malware type from threat name
func (s *Scanner) categorizeThreat(threatName string) string {
	lower := strings.ToLower(threatName)

	categories := map[string]string{
		"trojan":    "trojan",
		"backdoor":  "backdoor",
		"virus":     "virus",
		"worm":      "worm",
		"rootkit":   "rootkit",
		"ransomware": "ransomware",
		"exploit":   "exploit",
		"adware":    "adware",
		"spyware":   "spyware",
		"miner":     "cryptominer",
		"coinminer": "cryptominer",
		"downloader": "downloader",
		"dropper":   "dropper",
	}

	for keyword, category := range categories {
		if strings.Contains(lower, keyword) {
			return category
		}
	}

	return "malware"
}

// determineSeverity determines severity based on threat type
func (s *Scanner) determineSeverity(threatName string) string {
	lower := strings.ToLower(threatName)

	// Critical threats
	criticalKeywords := []string{"ransomware", "backdoor", "rootkit", "trojan"}
	for _, keyword := range criticalKeywords {
		if strings.Contains(lower, keyword) {
			return "critical"
		}
	}

	// High severity threats
	highKeywords := []string{"virus", "worm", "exploit", "miner", "coinminer"}
	for _, keyword := range highKeywords {
		if strings.Contains(lower, keyword) {
			return "high"
		}
	}

	// Medium severity
	mediumKeywords := []string{"adware", "spyware", "downloader", "dropper"}
	for _, keyword := range mediumKeywords {
		if strings.Contains(lower, keyword) {
			return "medium"
		}
	}

	// Default to high for unknown malware
	return "high"
}

// convertFindings converts MalwareFindings to scanner.Vuln format
func (s *Scanner) convertFindings(findings []MalwareFinding) []scanner.Vuln {
	var vulns []scanner.Vuln

	for _, finding := range findings {
		vulns = append(vulns, scanner.Vuln{
			ID:          fmt.Sprintf("MALWARE-%s", strings.ReplaceAll(finding.ThreatName, " ", "-")),
			Package:     finding.FilePath,
			Version:     "",
			FixVersion:  "Remove infected file",
			Severity:    strings.ToUpper(finding.Severity),
			Description: fmt.Sprintf("Malware detected: %s (%s)", finding.ThreatName, finding.ThreatType),
		})
	}

	return vulns
}

// extractImageLayers extracts container image layers to a directory
func (s *Scanner) extractImageLayers(ctx context.Context, digest, destDir string) error {
	log.Printf("[ClamAV] Extracting image layers for %s", digest)

	// Construct blob path
	blobPath := fmt.Sprintf("blobs/%s", digest)

	// Get blob from storage
	blobReader, err := s.storage.Reader(ctx, blobPath, 0)
	if err != nil {
		return fmt.Errorf("failed to get blob: %w", err)
	}
	defer blobReader.Close()

	// Save tarball
	tarPath := filepath.Join(destDir, "image.tar")
	tarFile, err := os.Create(tarPath)
	if err != nil {
		return fmt.Errorf("failed to create tar file: %w", err)
	}
	defer tarFile.Close()

	if _, err := io.Copy(tarFile, blobReader); err != nil {
		return fmt.Errorf("failed to write tar: %w", err)
	}

	// Extract tarball
	cmd := exec.CommandContext(ctx, "tar", "-xf", tarPath, "-C", destDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[ClamAV] Tar extraction output: %s", string(output))
		return fmt.Errorf("tar extraction failed: %w", err)
	}

	log.Printf("[ClamAV] Successfully extracted image to %s", destDir)
	return nil
}

// getVersion gets ClamAV version
func (s *Scanner) getVersion() string {
	if s.useDaemon {
		// Try to get version from clamd
		conn, err := s.connectToClamd()
		if err == nil {
			defer conn.Close()
			conn.Write([]byte("VERSION\n"))
			response := make([]byte, 1024)
			n, err := conn.Read(response)
			if err == nil {
				return strings.TrimSpace(string(response[:n]))
			}
		}
	}

	// Fallback: try clamscan --version
	cmd := exec.Command("clamscan", "--version")
	output, err := cmd.CombinedOutput()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 {
			return strings.TrimSpace(lines[0])
		}
	}

	return "unknown"
}
