package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var (
	force          bool
	deleteAllVersions bool
)

var unpublishCmd = &cobra.Command{
	Use:   "unpublish <package-name> [version]",
	Short: "Remove a package or version from the registry",
	Long: `Unpublish (delete) a package version or entire package from the registry.

WARNING: This action is irreversible!

Examples:
  # Remove specific version
  artifactadm unpublish --format npm express 4.18.2

  # Remove all versions of a package
  artifactadm unpublish --format npm express --all --force

  # Remove with confirmation
  artifactadm unpublish --format pypi requests 2.28.0 --force`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		packageName := args[0]
		version := ""
		if len(args) > 1 {
			version = args[1]
		}
		runUnpublish(packageName, version)
	},
}

func init() {
	rootCmd.AddCommand(unpublishCmd)
	unpublishCmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	unpublishCmd.Flags().BoolVar(&deleteAllVersions, "all", false, "Delete all versions of the package")
}

func runUnpublish(packageName, version string) {
	regURL := getRegistryURL()
	token := getAuthToken()
	fmt := getFormat()
	ns := getNamespace()

	// Confirmation prompt
	if !force {
		var confirm string
		if deleteAllVersions || version == "" {
			fmt.Printf("This will delete ALL versions of %s. Are you sure? (yes/no): ", packageName)
		} else {
			fmt.Printf("This will delete %s@%s. Are you sure? (yes/no): ", packageName, version)
		}
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	var url string
	if deleteAllVersions || version == "" {
		url = fmt.Sprintf("%s/api/v1/artifacts/%s/%s/%s", regURL, fmt, ns, packageName)
	} else {
		url = fmt.Sprintf("%s/api/v1/artifacts/%s/%s/%s/%s", regURL, fmt, ns, packageName, version)
	}

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
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

	if resp.StatusCode == 404 {
		fmt.Fprintf(os.Stderr, "Package or version not found\n")
		os.Exit(1)
	}

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "HTTP %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	if deleteAllVersions || version == "" {
		fmt.Printf("Successfully deleted all versions of %s\n", packageName)
	} else {
		fmt.Printf("Successfully deleted %s@%s\n", packageName, version)
	}
}
