package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "View registry statistics",
	Long:  `Display registry statistics including repositories, storage, and vulnerabilities`,
	Run:   runStats,
}

func init() {
	rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/stats")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var stats struct {
		TotalRepos    int    `json:"total_repos"`
		StorageUsed   string `json:"storage_used"`
		CriticalVulns int    `json:"critical_vulns"`
		PolicyBlocks  int    `json:"policy_blocks"`
	}

	if err := json.Unmarshal(data, &stats); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Registry Statistics")
	fmt.Println("===================")
	fmt.Printf("Total Repositories:   %d\n", stats.TotalRepos)
	fmt.Printf("Storage Used:         %s\n", stats.StorageUsed)
	fmt.Printf("Critical Vulns:       %d\n", stats.CriticalVulns)
	fmt.Printf("Policy Blocks:        %d\n", stats.PolicyBlocks)
}
