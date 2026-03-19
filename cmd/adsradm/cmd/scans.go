package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var scansCmd = &cobra.Command{
	Use:   "scans",
	Short: "Manage vulnerability scans",
	Long:  `List and view vulnerability scan reports`,
}

var listScansCmd = &cobra.Command{
	Use:   "list",
	Short: "List all vulnerability scans",
	Run:   runListScans,
}

var getScanCmd = &cobra.Command{
	Use:   "get [digest]",
	Short: "Get detailed scan report for a digest",
	Args:  cobra.ExactArgs(1),
	Run:   runGetScan,
}

var scanScanner string

func init() {
	rootCmd.AddCommand(scansCmd)
	scansCmd.AddCommand(listScansCmd)
	scansCmd.AddCommand(getScanCmd)

	getScanCmd.Flags().StringVar(&scanScanner, "scanner", "trivy", "Scanner name")
}

func runListScans(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/scans")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var scans []struct {
		Digest   string `json:"digest"`
		Scanner  string `json:"scanner"`
		Critical int    `json:"critical"`
		High     int    `json:"high"`
		Medium   int    `json:"medium"`
		Low      int    `json:"low"`
	}

	if err := json.Unmarshal(data, &scans); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(scans) == 0 {
		fmt.Println("No vulnerability scans found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DIGEST\tSCANNER\tCRITICAL\tHIGH\tMEDIUM\tLOW")
	for _, s := range scans {
		digest := s.Digest
		if len(digest) > 19 {
			digest = digest[:19] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\n", digest, s.Scanner, s.Critical, s.High, s.Medium, s.Low)
	}
	w.Flush()
}

func runGetScan(cmd *cobra.Command, args []string) {
	digest := args[0]
	client := NewAPIClient()

	data, err := client.Get(fmt.Sprintf("/api/v1/management/scans/%s?scanner=%s", digest, scanScanner))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Pretty print the JSON report
	var report interface{}
	if err := json.Unmarshal(data, &report); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	prettyJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(prettyJSON))
}
