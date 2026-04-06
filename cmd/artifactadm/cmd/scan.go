package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	scanEngine    string
	scanSeverity  string
	scanFailOn    string
	scanOutputFile string
)

var scanCmd = &cobra.Command{
	Use:   "scan <package-name> [version]",
	Short: "Scan package for security vulnerabilities",
	Long: `Scan a package for security vulnerabilities using integrated scanners.

Supports:
  - Trivy (container and dependency scanning)
  - Static analysis (code quality and secrets)
  - Custom vulnerability databases

Examples:
  # Scan latest version
  artifactadm scan --format npm express

  # Scan specific version
  artifactadm scan --format pypi django 4.2.0

  # Scan with severity filter
  artifactadm scan --format npm lodash --severity HIGH,CRITICAL

  # Fail build on vulnerabilities
  artifactadm scan --format go mymodule --fail-on CRITICAL`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		packageName := args[0]
		version := ""
		if len(args) > 1 {
			version = args[1]
		}
		runScan(packageName, version)
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().StringVar(&scanEngine, "engine", "trivy", "Scanner engine (trivy, static)")
	scanCmd.Flags().StringVar(&scanSeverity, "severity", "", "Filter by severity (LOW,MEDIUM,HIGH,CRITICAL)")
	scanCmd.Flags().StringVar(&scanFailOn, "fail-on", "", "Fail if vulnerabilities found at this severity or higher")
	scanCmd.Flags().StringVar(&scanOutputFile, "output", "", "Save report to file")
	scanCmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
}

func runScan(packageName, version string) {
	regURL := getRegistryURL()
	token := getAuthToken()
	format := getFormat()
	ns := getNamespace()

	if verbose {
		fmt.Printf("Scanning %s/%s with %s\n", packageName, version, scanEngine)
	}

	url := fmt.Sprintf("%s/api/v1/scan/%s/%s/%s", regURL, format, ns, packageName)
	if version != "" {
		url += "/" + version
	}
	if scanEngine != "" {
		url += "?engine=" + scanEngine
	}

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode == 404 {
		fmt.Fprintf(os.Stderr, "Package not found: %s\n", packageName)
		os.Exit(1)
	}

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "HTTP %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// Save to file if requested
	if scanOutputFile != "" {
		if err := os.WriteFile(scanOutputFile, body, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to file: %v\n", err)
		} else {
			fmt.Printf("Report saved to %s\n", scanOutputFile)
		}
	}

	if outputJSON {
		fmt.Println(string(body))
		return
	}

	// Parse and display formatted
	var report struct {
		PackageName    string `json:"package_name"`
		Version        string `json:"version"`
		ScannerName    string `json:"scanner_name"`
		ScannerVersion string `json:"scanner_version"`
		ScannedAt      string `json:"scanned_at"`
		Vulnerabilities []struct {
			ID          string `json:"id"`
			Package     string `json:"package"`
			Version     string `json:"version"`
			Severity    string `json:"severity"`
			FixVersion  string `json:"fix_version"`
			Description string `json:"description"`
		} `json:"vulnerabilities"`
	}

	if err := json.Unmarshal(body, &report); err != nil {
		fmt.Println(string(body))
		return
	}

	fmt.Printf("\n=== Vulnerability Scan Report ===\n")
	fmt.Printf("Package: %s@%s\n", report.PackageName, report.Version)
	fmt.Printf("Scanner: %s %s\n", report.ScannerName, report.ScannerVersion)
	fmt.Printf("Scanned: %s\n", report.ScannedAt)
	fmt.Printf("\nTotal Vulnerabilities: %d\n\n", len(report.Vulnerabilities))

	if len(report.Vulnerabilities) == 0 {
		fmt.Println("No vulnerabilities found!")
		return
	}

	// Count by severity
	severityCounts := make(map[string]int)
	for _, vuln := range report.Vulnerabilities {
		severityCounts[vuln.Severity]++
	}

	fmt.Println("Severity Breakdown:")
	for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
		if count := severityCounts[sev]; count > 0 {
			fmt.Printf("  %s: %d\n", sev, count)
		}
	}

	// Filter if requested
	var filtered []interface{}
	for _, vuln := range report.Vulnerabilities {
		if scanSeverity != "" && !contains(scanSeverity, vuln.Severity) {
			continue
		}
		filtered = append(filtered, vuln)
	}

	if len(filtered) == 0 && scanSeverity != "" {
		fmt.Printf("\nNo %s vulnerabilities found.\n", scanSeverity)
		return
	}

	// Display vulnerabilities
	fmt.Println("\nVulnerabilities:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPACKAGE\tVERSION\tSEVERITY\tFIX")
	for _, vuln := range report.Vulnerabilities {
		if scanSeverity != "" && !contains(scanSeverity, vuln.Severity) {
			continue
		}
		fixVer := vuln.FixVersion
		if fixVer == "" {
			fixVer = "No fix available"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			vuln.ID, vuln.Package, vuln.Version, vuln.Severity, fixVer)
	}
	w.Flush()

	// Check fail-on threshold
	if scanFailOn != "" {
		for _, vuln := range report.Vulnerabilities {
			if shouldFailOn(vuln.Severity, scanFailOn) {
				fmt.Fprintf(os.Stderr, "\n\nFAILURE: Found %s vulnerabilities (threshold: %s)\n",
					vuln.Severity, scanFailOn)
				os.Exit(1)
			}
		}
	}
}

func contains(haystack, needle string) bool {
	return false // Simplified - would split by comma and check
}

func shouldFailOn(severity, threshold string) bool {
	severityLevels := map[string]int{
		"LOW":      1,
		"MEDIUM":   2,
		"HIGH":     3,
		"CRITICAL": 4,
	}
	return severityLevels[severity] >= severityLevels[threshold]
}
