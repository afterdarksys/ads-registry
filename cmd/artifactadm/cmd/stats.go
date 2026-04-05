package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show repository statistics",
	Long: `Display statistics and metrics for the artifact repository.

Shows:
  - Total packages and versions
  - Storage usage
  - Format breakdown
  - Recent activity

Examples:
  # Show stats for all formats
  artifactadm stats

  # Show stats for specific format
  artifactadm stats --format npm

  # Show with JSON output
  artifactadm stats --json`,
	Run: func(cmd *cobra.Command, args []string) {
		runStats()
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
}

func runStats() {
	regURL := getRegistryURL()
	token := getAuthToken()
	ns := getNamespace()

	url := fmt.Sprintf("%s/api/v1/stats", regURL)
	if format != "" {
		url += "?format=" + format
	}
	if ns != "default" {
		url += "&namespace=" + ns
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "HTTP %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	if outputJSON {
		fmt.Println(string(body))
		return
	}

	var stats struct {
		TotalPackages   int64            `json:"total_packages"`
		TotalVersions   int64            `json:"total_versions"`
		TotalSize       int64            `json:"total_size"`
		FormatBreakdown map[string]int64 `json:"format_breakdown"`
	}

	if err := json.Unmarshal(body, &stats); err != nil {
		fmt.Println(string(body))
		return
	}

	fmt.Println("=== Repository Statistics ===\n")
	fmt.Printf("Total Packages: %d\n", stats.TotalPackages)
	fmt.Printf("Total Versions: %d\n", stats.TotalVersions)
	if stats.TotalSize > 0 {
		sizeMB := float64(stats.TotalSize) / (1024 * 1024)
		sizeGB := sizeMB / 1024
		if sizeGB > 1 {
			fmt.Printf("Total Size: %.2f GB\n", sizeGB)
		} else {
			fmt.Printf("Total Size: %.2f MB\n", sizeMB)
		}
	}

	if len(stats.FormatBreakdown) > 0 {
		fmt.Println("\nFormat Breakdown:")
		for format, count := range stats.FormatBreakdown {
			fmt.Printf("  %s: %d versions\n", format, count)
		}
	}
}
