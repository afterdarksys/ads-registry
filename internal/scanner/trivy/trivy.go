package trivy

import (
	"context"
	"log"

	"github.com/ryan/ads-registry/internal/scanner"
)

// EmbeddedScanner wraps the Trivy engine API
type EmbeddedScanner struct {
	cacheDir string
}

func New(cacheDir string) *EmbeddedScanner {
	log.Printf("Initializing Embedded Trivy Scanner (Cache: %s)", cacheDir)
	return &EmbeddedScanner{
		cacheDir: cacheDir,
	}
}

func (t *EmbeddedScanner) Name() string {
	return "trivy-embedded"
}

// Scan downloads the layers locally and analyzes them for CVEs and Secrets.
func (t *EmbeddedScanner) Scan(ctx context.Context, namespace, repo, digest string) (*scanner.Report, error) {
	log.Printf("[Trivy] Scanning %s/%s@%s...", namespace, repo, digest)

	// Since embedding Trivy natively takes significant setup (fetching the Vulnerability DB,
	// unpacking tarballs, linking the analysis modules), this serves as the structural Hook.
	// In a complete enterprise build, `gihtub.com/aquasecurity/trivy/pkg/scanner` is invoked here.

	// Mocking a successful scan for the MVP architecture demonstration
	return &scanner.Report{
		Digest:         digest,
		ScannerName:    "trivy-embedded",
		ScannerVersion: "0.48.0", // mock version
		Vulnerabilities: []scanner.Vuln{
			{
				ID:          "CVE-2023-MOCK",
				Package:     "demo-lib",
				Version:     "1.0.0",
				FixVersion:  "1.0.1",
				Severity:    "HIGH",
				Description: "This is a mock vulnerability demonstrating the Trivy scanner interface",
			},
		},
	}, nil
}
