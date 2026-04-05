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
	showMetadata bool
	showBlobs    bool
)

var infoCmd = &cobra.Command{
	Use:   "info <package-name> [version]",
	Short: "Show detailed package information",
	Long: `Display detailed information about a package including metadata, versions, and blobs.

Examples:
  # Show info for latest version
  artifactadm info --format npm express

  # Show info for specific version
  artifactadm info --format npm express 4.18.2

  # Show with full metadata
  artifactadm info --format pypi requests --metadata`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		packageName := args[0]
		version := ""
		if len(args) > 1 {
			version = args[1]
		}
		runInfo(packageName, version)
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
	infoCmd.Flags().BoolVar(&showMetadata, "metadata", false, "Show full metadata")
	infoCmd.Flags().BoolVar(&showBlobs, "blobs", false, "Show blob details")
	infoCmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
}

func runInfo(packageName, version string) {
	regURL := getRegistryURL()
	token := getAuthToken()
	fmt := getFormat()
	ns := getNamespace()

	url := fmt.Sprintf("%s/api/v1/artifacts/%s/%s/%s", regURL, fmt, ns, packageName)
	if version != "" {
		url += "/" + version
	}

	req, err := http.NewRequest("GET", url, nil)
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

	if outputJSON {
		fmt.Println(string(body))
		return
	}

	// Parse and display formatted
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Package: %s\n", packageName)
	if v, ok := result["version"]; ok {
		fmt.Printf("Version: %s\n", v)
	}
	if created, ok := result["created_at"]; ok {
		fmt.Printf("Created: %s\n", created)
	}
	if format, ok := result["format"]; ok {
		fmt.Printf("Format: %s\n", format)
	}
	if namespace, ok := result["namespace"]; ok {
		fmt.Printf("Namespace: %s\n", namespace)
	}

	// Show metadata if requested
	if showMetadata {
		if metadata, ok := result["metadata"]; ok {
			fmt.Println("\nMetadata:")
			metaJSON, _ := json.MarshalIndent(metadata, "  ", "  ")
			fmt.Println("  " + string(metaJSON))
		}
	}

	// Show blobs if requested or available
	if blobs, ok := result["blobs"].([]interface{}); ok && (showBlobs || len(blobs) > 0) {
		fmt.Println("\nBlobs:")
		for _, blob := range blobs {
			if blobMap, ok := blob.(map[string]interface{}); ok {
				fmt.Printf("  - File: %s\n", blobMap["file_name"])
				fmt.Printf("    Digest: %s\n", blobMap["blob_digest"])
				if created, ok := blobMap["created_at"]; ok {
					fmt.Printf("    Created: %s\n", created)
				}
			}
		}
	}
}
