package supplychain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Analyzer performs supply chain security analysis
type Analyzer struct {
	sbomValidator        *SBOMValidator
	provenanceValidator  *ProvenanceValidator
	signatureValidator   *SignatureValidator
	dependencyAnalyzer   *DependencyAnalyzer
}

// NewAnalyzer creates a new supply chain analyzer
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		sbomValidator:       NewSBOMValidator(),
		provenanceValidator: NewProvenanceValidator(),
		signatureValidator:  NewSignatureValidator(),
		dependencyAnalyzer:  NewDependencyAnalyzer(),
	}
}

// ============================================================================
// SUPPLY CHAIN ANALYSIS RESULTS
// ============================================================================

// GapAnalysisReport contains the comprehensive supply chain analysis
type GapAnalysisReport struct {
	ImageDigest   string    `json:"image_digest"`
	AnalyzedAt    time.Time `json:"analyzed_at"`
	OverallScore  int       `json:"overall_score"`  // 0-100
	MaturityLevel string    `json:"maturity_level"` // none, basic, intermediate, advanced, exemplary

	// Component Analysis
	SBOM        SBOMAnalysis        `json:"sbom"`
	Provenance  ProvenanceAnalysis  `json:"provenance"`
	Signatures  SignatureAnalysis   `json:"signatures"`
	Dependencies DependencyAnalysis `json:"dependencies"`

	// Gap Summary
	Gaps         []Gap         `json:"gaps"`
	Warnings     []Warning     `json:"warnings"`
	Recommendations []string   `json:"recommendations"`

	// Compliance
	ComplianceStatus ComplianceStatus `json:"compliance"`
}

// SBOMAnalysis contains SBOM-related findings
type SBOMAnalysis struct {
	Present          bool      `json:"present"`
	Format           string    `json:"format"`            // spdx, cyclonedx, syft
	Version          string    `json:"version"`
	ComponentCount   int       `json:"component_count"`
	PackageManagers  []string  `json:"package_managers"`  // npm, pip, maven, go, etc.

	// Quality metrics
	HasLicenses      bool      `json:"has_licenses"`
	HasVulnData      bool      `json:"has_vulnerability_data"`
	HasRelationships bool      `json:"has_relationships"`
	Completeness     int       `json:"completeness"`      // 0-100

	// Timestamps
	CreatedAt        time.Time `json:"created_at,omitempty"`
	ValidatedAt      time.Time `json:"validated_at"`

	// Issues
	Issues           []string  `json:"issues,omitempty"`
}

// ProvenanceAnalysis contains provenance/attestation findings
type ProvenanceAnalysis struct {
	Present          bool      `json:"present"`
	Format           string    `json:"format"`    // in-toto, slsa
	SLSALevel        int       `json:"slsa_level"` // 0-4

	// Build information
	Builder          string    `json:"builder,omitempty"`
	BuildType        string    `json:"build_type,omitempty"`
	BuildInvocation  string    `json:"build_invocation,omitempty"`

	// Source information
	SourceRepo       string    `json:"source_repo,omitempty"`
	SourceCommit     string    `json:"source_commit,omitempty"`
	SourceBranch     string    `json:"source_branch,omitempty"`

	// Verification
	Verified         bool      `json:"verified"`
	VerifiedBy       string    `json:"verified_by,omitempty"`
	VerifiedAt       time.Time `json:"verified_at,omitempty"`

	// Issues
	Issues           []string  `json:"issues,omitempty"`
}

// SignatureAnalysis contains signature verification findings
type SignatureAnalysis struct {
	Signed           bool      `json:"signed"`
	SignatureCount   int       `json:"signature_count"`

	// Signature details
	Signatures       []Signature `json:"signatures,omitempty"`

	// Cosign/Sigstore specific
	CosignVerified   bool      `json:"cosign_verified"`
	RekorEntry       string    `json:"rekor_entry,omitempty"`
	FulcioIssuer     string    `json:"fulcio_issuer,omitempty"`

	// Trust
	TrustedSigners   []string  `json:"trusted_signers,omitempty"`
	UnknownSigners   []string  `json:"unknown_signers,omitempty"`

	// Issues
	Issues           []string  `json:"issues,omitempty"`
}

// Signature represents a single signature
type Signature struct {
	Signer      string    `json:"signer"`
	SignedAt    time.Time `json:"signed_at"`
	Algorithm   string    `json:"algorithm"`
	Verified    bool      `json:"verified"`
	Certificate string    `json:"certificate,omitempty"`
}

// DependencyAnalysis contains dependency risk analysis
type DependencyAnalysis struct {
	TotalDependencies    int      `json:"total_dependencies"`
	DirectDependencies   int      `json:"direct_dependencies"`
	TransitiveDependencies int    `json:"transitive_dependencies"`

	// Risk factors
	HighRiskPackages     []RiskyPackage `json:"high_risk_packages,omitempty"`
	Typosquatting        []string       `json:"typosquatting,omitempty"`
	UnmaintainedPackages []string       `json:"unmaintained_packages,omitempty"`

	// Licensing
	UnknownLicenses      int            `json:"unknown_licenses"`
	ConflictingLicenses  []string       `json:"conflicting_licenses,omitempty"`

	// Update status
	OutdatedPackages     int            `json:"outdated_packages"`
	SecurityUpdates      int            `json:"security_updates_available"`
}

// RiskyPackage represents a package with known issues
type RiskyPackage struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Ecosystem   string   `json:"ecosystem"`
	Reasons     []string `json:"reasons"`
	RiskScore   int      `json:"risk_score"` // 0-100
}

// Gap represents a missing security control
type Gap struct {
	Category    string `json:"category"`    // sbom, provenance, signature, dependency
	Severity    string `json:"severity"`    // critical, high, medium, low
	Description string `json:"description"`
	Impact      string `json:"impact"`
	Remediation string `json:"remediation"`
}

// Warning represents a potential issue
type Warning struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// ComplianceStatus tracks compliance with various frameworks
type ComplianceStatus struct {
	SLSA         SLSACompliance     `json:"slsa"`
	SSDF         SSDFCompliance     `json:"ssdf"`      // NIST Secure Software Development Framework
	CISBenchmark CISCompliance      `json:"cis"`
	Custom       map[string]bool    `json:"custom,omitempty"`
}

// SLSACompliance tracks SLSA level compliance
type SLSACompliance struct {
	Level        int      `json:"level"`        // 0-4
	Requirements []string `json:"requirements"` // Met requirements
	Missing      []string `json:"missing"`      // Missing requirements
}

// SSDFCompliance tracks NIST SSDF compliance
type SSDFCompliance struct {
	Compliant    bool     `json:"compliant"`
	PracticesMet []string `json:"practices_met"`
	GapsPractices []string `json:"gaps_practices"`
}

// CISCompliance tracks CIS Benchmark compliance
type CISCompliance struct {
	Level        int      `json:"level"`        // 1 or 2
	PassedChecks int      `json:"passed_checks"`
	FailedChecks int      `json:"failed_checks"`
	Score        int      `json:"score"`        // 0-100
}

// ============================================================================
// ANALYSIS FUNCTIONS
// ============================================================================

// AnalyzeImage performs comprehensive supply chain analysis
func (a *Analyzer) AnalyzeImage(ctx context.Context, imageDigest string, manifestData []byte) (*GapAnalysisReport, error) {
	report := &GapAnalysisReport{
		ImageDigest: imageDigest,
		AnalyzedAt:  time.Now(),
		Gaps:        []Gap{},
		Warnings:    []Warning{},
		Recommendations: []string{},
	}

	// Analyze SBOM
	sbomAnalysis, err := a.sbomValidator.Analyze(ctx, imageDigest)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze SBOM: %w", err)
	}
	report.SBOM = sbomAnalysis

	// Analyze Provenance
	provenanceAnalysis, err := a.provenanceValidator.Analyze(ctx, imageDigest)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze provenance: %w", err)
	}
	report.Provenance = provenanceAnalysis

	// Analyze Signatures
	signatureAnalysis, err := a.signatureValidator.Analyze(ctx, imageDigest)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze signatures: %w", err)
	}
	report.Signatures = signatureAnalysis

	// Analyze Dependencies
	dependencyAnalysis, err := a.dependencyAnalyzer.Analyze(ctx, imageDigest)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze dependencies: %w", err)
	}
	report.Dependencies = dependencyAnalysis

	// Identify gaps
	a.identifyGaps(report)

	// Calculate maturity level
	a.calculateMaturityLevel(report)

	// Generate recommendations
	a.generateRecommendations(report)

	// Check compliance
	a.checkCompliance(report)

	return report, nil
}

// identifyGaps identifies missing supply chain security controls
func (a *Analyzer) identifyGaps(report *GapAnalysisReport) {
	// SBOM gaps
	if !report.SBOM.Present {
		report.Gaps = append(report.Gaps, Gap{
			Category:    "sbom",
			Severity:    "high",
			Description: "No SBOM (Software Bill of Materials) found",
			Impact:      "Cannot track components, licenses, or vulnerabilities effectively",
			Remediation: "Generate SBOM using Syft, CycloneDX, or SPDX tools during build",
		})
	} else if report.SBOM.Completeness < 70 {
		report.Gaps = append(report.Gaps, Gap{
			Category:    "sbom",
			Severity:    "medium",
			Description: fmt.Sprintf("SBOM completeness is %d%% (< 70%%)", report.SBOM.Completeness),
			Impact:      "Incomplete dependency tracking may miss vulnerabilities",
			Remediation: "Improve SBOM generation to include all dependencies and metadata",
		})
	}

	// Provenance gaps
	if !report.Provenance.Present {
		report.Gaps = append(report.Gaps, Gap{
			Category:    "provenance",
			Severity:    "critical",
			Description: "No provenance attestation found",
			Impact:      "Cannot verify build integrity or source authenticity",
			Remediation: "Implement SLSA provenance using GitHub Actions, GitLab CI, or Tekton",
		})
	} else if report.Provenance.SLSALevel < 3 {
		report.Gaps = append(report.Gaps, Gap{
			Category:    "provenance",
			Severity:    "medium",
			Description: fmt.Sprintf("SLSA level %d (< 3)", report.Provenance.SLSALevel),
			Impact:      "Build process may be vulnerable to tampering",
			Remediation: "Upgrade to SLSA Level 3 with isolated build environment",
		})
	}

	// Signature gaps
	if !report.Signatures.Signed {
		report.Gaps = append(report.Gaps, Gap{
			Category:    "signature",
			Severity:    "high",
			Description: "Image is not signed",
			Impact:      "Cannot verify image authenticity or detect tampering",
			Remediation: "Sign images using Cosign (keyless or key-based)",
		})
	}

	// Dependency gaps
	if len(report.Dependencies.HighRiskPackages) > 0 {
		report.Gaps = append(report.Gaps, Gap{
			Category:    "dependency",
			Severity:    "high",
			Description: fmt.Sprintf("%d high-risk packages detected", len(report.Dependencies.HighRiskPackages)),
			Impact:      "Potential security vulnerabilities or supply chain attacks",
			Remediation: "Review and replace high-risk dependencies",
		})
	}

	if len(report.Dependencies.Typosquatting) > 0 {
		report.Gaps = append(report.Gaps, Gap{
			Category:    "dependency",
			Severity:    "critical",
			Description: fmt.Sprintf("%d potential typosquatting packages found", len(report.Dependencies.Typosquatting)),
			Impact:      "May contain malicious code mimicking legitimate packages",
			Remediation: "Remove typosquatted packages and verify correct package names",
		})
	}
}

// calculateMaturityLevel determines the supply chain security maturity level
func (a *Analyzer) calculateMaturityLevel(report *GapAnalysisReport) {
	score := 0

	// SBOM scoring (30 points)
	if report.SBOM.Present {
		score += 15
		score += (report.SBOM.Completeness * 15 / 100)
	}

	// Provenance scoring (30 points)
	if report.Provenance.Present {
		score += 10
		if report.Provenance.Verified {
			score += 10
		}
		score += (report.Provenance.SLSALevel * 5 / 2) // SLSA 0-4 → 0-10 points
	}

	// Signature scoring (25 points)
	if report.Signatures.Signed {
		score += 15
		if report.Signatures.CosignVerified {
			score += 10
		}
	}

	// Dependency scoring (15 points)
	if report.Dependencies.TotalDependencies > 0 {
		riskFactor := len(report.Dependencies.HighRiskPackages) * 100 / report.Dependencies.TotalDependencies
		score += (100 - riskFactor) * 15 / 100
	} else {
		score += 15 // No dependencies = no risk
	}

	report.OverallScore = score

	// Determine maturity level
	switch {
	case score >= 90:
		report.MaturityLevel = "exemplary"
	case score >= 70:
		report.MaturityLevel = "advanced"
	case score >= 50:
		report.MaturityLevel = "intermediate"
	case score >= 30:
		report.MaturityLevel = "basic"
	default:
		report.MaturityLevel = "none"
	}
}

// generateRecommendations creates actionable recommendations
func (a *Analyzer) generateRecommendations(report *GapAnalysisReport) {
	if !report.SBOM.Present {
		report.Recommendations = append(report.Recommendations,
			"Integrate Syft or CycloneDX into your build pipeline to generate SBOMs automatically")
	}

	if !report.Provenance.Present {
		report.Recommendations = append(report.Recommendations,
			"Use SLSA provenance generators (GitHub Actions SLSA builder, GitLab SLSA, or Tekton Chains)")
	}

	if !report.Signatures.Signed {
		report.Recommendations = append(report.Recommendations,
			"Sign images using Cosign with keyless signing (OIDC) for ease of use")
	}

	if report.Dependencies.SecurityUpdates > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Update %d packages with available security patches", report.Dependencies.SecurityUpdates))
	}

	if len(report.Dependencies.HighRiskPackages) > 0 {
		report.Recommendations = append(report.Recommendations,
			"Review and potentially replace high-risk dependencies with safer alternatives")
	}

	if report.Provenance.SLSALevel < 3 {
		report.Recommendations = append(report.Recommendations,
			"Upgrade to SLSA Level 3 for stronger build integrity guarantees")
	}
}

// checkCompliance evaluates compliance with security frameworks
func (a *Analyzer) checkCompliance(report *GapAnalysisReport) {
	// SLSA compliance
	report.ComplianceStatus.SLSA = SLSACompliance{
		Level:        report.Provenance.SLSALevel,
		Requirements: []string{},
		Missing:      []string{},
	}

	if report.Provenance.SLSALevel >= 1 {
		report.ComplianceStatus.SLSA.Requirements = append(report.ComplianceStatus.SLSA.Requirements,
			"Build process is fully scripted/automated")
	} else {
		report.ComplianceStatus.SLSA.Missing = append(report.ComplianceStatus.SLSA.Missing,
			"Build process must be fully scripted/automated")
	}

	if report.Provenance.SLSALevel >= 2 {
		report.ComplianceStatus.SLSA.Requirements = append(report.ComplianceStatus.SLSA.Requirements,
			"Provenance is available and authenticated")
	} else {
		report.ComplianceStatus.SLSA.Missing = append(report.ComplianceStatus.SLSA.Missing,
			"Provenance must be available and authenticated")
	}

	if report.Provenance.SLSALevel >= 3 {
		report.ComplianceStatus.SLSA.Requirements = append(report.ComplianceStatus.SLSA.Requirements,
			"Build service is hardened and isolated")
	} else {
		report.ComplianceStatus.SLSA.Missing = append(report.ComplianceStatus.SLSA.Missing,
			"Build service must be hardened and isolated")
	}

	// NIST SSDF compliance
	practicesMet := 0
	totalPractices := 4

	if report.SBOM.Present {
		practicesMet++
		report.ComplianceStatus.SSDF.PracticesMet = append(report.ComplianceStatus.SSDF.PracticesMet,
			"PO.3: Obtain software components from trusted sources")
	} else {
		report.ComplianceStatus.SSDF.GapsPractices = append(report.ComplianceStatus.SSDF.GapsPractices,
			"PO.3: Obtain software components from trusted sources")
	}

	if report.Provenance.Present {
		practicesMet++
		report.ComplianceStatus.SSDF.PracticesMet = append(report.ComplianceStatus.SSDF.PracticesMet,
			"PS.2: Create and maintain development environment")
	} else {
		report.ComplianceStatus.SSDF.GapsPractices = append(report.ComplianceStatus.SSDF.GapsPractices,
			"PS.2: Create and maintain development environment")
	}

	if report.Signatures.Signed {
		practicesMet++
		report.ComplianceStatus.SSDF.PracticesMet = append(report.ComplianceStatus.SSDF.PracticesMet,
			"PW.4: Create and maintain integrity verification mechanism")
	} else {
		report.ComplianceStatus.SSDF.GapsPractices = append(report.ComplianceStatus.SSDF.GapsPractices,
			"PW.4: Create and maintain integrity verification mechanism")
	}

	if len(report.Dependencies.HighRiskPackages) == 0 {
		practicesMet++
		report.ComplianceStatus.SSDF.PracticesMet = append(report.ComplianceStatus.SSDF.PracticesMet,
			"RV.1: Identify and confirm vulnerabilities")
	} else {
		report.ComplianceStatus.SSDF.GapsPractices = append(report.ComplianceStatus.SSDF.GapsPractices,
			"RV.1: Identify and confirm vulnerabilities")
	}

	report.ComplianceStatus.SSDF.Compliant = (practicesMet == totalPractices)
}

// ToJSON converts the report to JSON
func (r *GapAnalysisReport) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
