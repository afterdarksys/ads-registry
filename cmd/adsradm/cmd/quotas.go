package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var quotasCmd = &cobra.Command{
	Use:   "quotas",
	Short: "Manage namespace quotas",
	Long:  `List and set storage quotas for namespaces`,
}

var listQuotasCmd = &cobra.Command{
	Use:   "list",
	Short: "List all quotas",
	Run:   runListQuotas,
}

var setQuotaCmd = &cobra.Command{
	Use:   "set [namespace] [limit-bytes]",
	Short: "Set quota for a namespace",
	Args:  cobra.ExactArgs(2),
	Run:   runSetQuota,
}

func init() {
	rootCmd.AddCommand(quotasCmd)
	quotasCmd.AddCommand(listQuotasCmd)
	quotasCmd.AddCommand(setQuotaCmd)
}

func runListQuotas(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/quotas")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var quotas []struct {
		Namespace  string `json:"namespace"`
		LimitBytes int64  `json:"limit_bytes"`
		UsedBytes  int64  `json:"used_bytes"`
	}

	if err := json.Unmarshal(data, &quotas); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(quotas) == 0 {
		fmt.Println("No quotas configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tUSED\tLIMIT\tUSAGE %")
	for _, q := range quotas {
		usedStr := formatBytes(q.UsedBytes)
		limitStr := formatBytes(q.LimitBytes)
		usagePercent := float64(q.UsedBytes) / float64(q.LimitBytes) * 100
		fmt.Fprintf(w, "%s\t%s\t%s\t%.1f%%\n", q.Namespace, usedStr, limitStr, usagePercent)
	}
	w.Flush()
}

func runSetQuota(cmd *cobra.Command, args []string) {
	namespace := args[0]
	var limitBytes int64
	if _, err := fmt.Sscanf(args[1], "%d", &limitBytes); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid limit bytes: %v\n", err)
		os.Exit(1)
	}

	client := NewAPIClient()

	payload := map[string]interface{}{
		"namespace":   namespace,
		"limit_bytes": limitBytes,
	}

	_, err := client.Post("/api/v1/management/quotas", payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Quota set for namespace '%s': %s\n", namespace, formatBytes(limitBytes))
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
