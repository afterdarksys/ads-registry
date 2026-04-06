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
	listAll    bool
	listLimit  int
	outputJSON bool
)

var listCmd = &cobra.Command{
	Use:   "list [package-name]",
	Short: "List packages or package versions",
	Long: `List available packages or versions of a specific package.

Examples:
  # List all packages for a format
  artifactadm list --format npm

  # List versions of a specific package
  artifactadm list --format npm express

  # List all with JSON output
  artifactadm list --format pypi --json`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var packageName string
		if len(args) > 0 {
			packageName = args[0]
		}
		runList(packageName)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVar(&listAll, "all", false, "List all packages (not just names)")
	listCmd.Flags().IntVar(&listLimit, "limit", 100, "Maximum number of results")
	listCmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
}

func runList(packageName string) {
	regURL := getRegistryURL()
	token := getAuthToken()
	format := getFormat()
	ns := getNamespace()

	if verbose {
		fmt.Printf("Listing %s packages from %s\n", format, regURL)
	}

	var url string
	if packageName != "" {
		// List versions of specific package
		url = fmt.Sprintf("%s/api/v1/artifacts/%s/%s/%s", regURL, format, ns, packageName)
	} else {
		// List all packages
		url = fmt.Sprintf("%s/api/v1/artifacts/%s/%s?limit=%d", regURL, format, ns, listLimit)
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

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "HTTP %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	if outputJSON {
		fmt.Println(string(body))
		return
	}

	// Parse and display as table
	var result struct {
		Packages []struct {
			Name      string   `json:"name"`
			Versions  []string `json:"versions"`
			Latest    string   `json:"latest"`
			CreatedAt string   `json:"created_at"`
		} `json:"packages"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		// Fallback to raw display
		fmt.Println(string(body))
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if packageName != "" {
		fmt.Fprintln(w, "VERSION\tCREATED")
		for _, pkg := range result.Packages {
			for _, version := range pkg.Versions {
				fmt.Fprintf(w, "%s\t%s\n", version, pkg.CreatedAt)
			}
		}
	} else {
		fmt.Fprintln(w, "PACKAGE\tLATEST\tVERSIONS\tCREATED")
		for _, pkg := range result.Packages {
			versionCount := len(pkg.Versions)
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", pkg.Name, pkg.Latest, versionCount, pkg.CreatedAt)
		}
	}
	w.Flush()
}
