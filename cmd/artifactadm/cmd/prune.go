package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var (
	pruneKeepVersions int
	pruneOlderThan    string
	pruneDryRun       bool
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Clean up old package versions",
	Long: `Remove old or unused package versions to free up storage space.

Strategies:
  - Keep N most recent versions (--keep)
  - Delete versions older than date (--older-than)
  - Dry run mode to preview changes (--dry-run)

Examples:
  # Keep only 5 most recent versions
  artifactadm prune --format npm --keep 5

  # Delete versions older than 90 days
  artifactadm prune --format pypi --older-than 90d

  # Preview what would be deleted
  artifactadm prune --format helm --keep 3 --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		runPrune()
	},
}

func init() {
	rootCmd.AddCommand(pruneCmd)
	pruneCmd.Flags().IntVar(&pruneKeepVersions, "keep", 5, "Number of recent versions to keep")
	pruneCmd.Flags().StringVar(&pruneOlderThan, "older-than", "", "Delete versions older than duration (e.g., 90d, 6m)")
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "Preview changes without deleting")
}

func runPrune() {
	regURL := getRegistryURL()
	token := getAuthToken()
	format := getFormat()
	ns := getNamespace()

	if verbose {
		fmt.Printf("Pruning %s packages in namespace %s\n", format, ns)
		fmt.Printf("Keep versions: %d, Dry run: %v\n", pruneKeepVersions, pruneDryRun)
	}

	url := fmt.Sprintf("%s/api/v1/prune/%s/%s?keep=%d&dry_run=%v",
		regURL, format, ns, pruneKeepVersions, pruneDryRun)

	if pruneOlderThan != "" {
		url += "&older_than=" + pruneOlderThan
	}

	req, err := http.NewRequest("POST", url, nil)
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

	var result struct {
		DryRun         bool     `json:"dry_run"`
		PackagesScanned int     `json:"packages_scanned"`
		VersionsDeleted int     `json:"versions_deleted"`
		SpaceFreed     int64    `json:"space_freed"`
		DeletedVersions []struct {
			Package string `json:"package"`
			Version string `json:"version"`
			Size    int64  `json:"size"`
		} `json:"deleted_versions"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Println(string(body))
		return
	}

	if result.DryRun {
		fmt.Println("=== DRY RUN MODE - No changes made ===\n")
	}

	fmt.Printf("Packages scanned: %d\n", result.PackagesScanned)
	fmt.Printf("Versions to delete: %d\n", result.VersionsDeleted)
	fmt.Printf("Space to free: %.2f MB\n\n", float64(result.SpaceFreed)/(1024*1024))

	if len(result.DeletedVersions) > 0 {
		fmt.Println("Versions that would be deleted:")
		for _, v := range result.DeletedVersions {
			sizeMB := float64(v.Size) / (1024 * 1024)
			fmt.Printf("  - %s@%s (%.2f MB)\n", v.Package, v.Version, sizeMB)
		}
	}

	if result.DryRun {
		fmt.Println("\nRun without --dry-run to apply changes.")
	} else {
		fmt.Println("\nPrune completed successfully!")
	}
}
