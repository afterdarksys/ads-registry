package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var upstreamsCmd = &cobra.Command{
	Use:   "upstreams",
	Short: "Manage upstream registries",
	Long:  `List and view upstream registry configurations`,
}

var listUpstreamsCmd = &cobra.Command{
	Use:   "list",
	Short: "List all upstream registries",
	Run:   runListUpstreams,
}

func init() {
	rootCmd.AddCommand(upstreamsCmd)
	upstreamsCmd.AddCommand(listUpstreamsCmd)
}

func runListUpstreams(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/upstreams")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var upstreams []struct {
		Name        string    `json:"name"`
		Type        string    `json:"type"`
		Endpoint    string    `json:"endpoint"`
		Region      string    `json:"region"`
		TokenExpiry time.Time `json:"token_expiry"`
	}

	if err := json.Unmarshal(data, &upstreams); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(upstreams) == 0 {
		fmt.Println("No upstream registries configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tREGION\tENDPOINT\tTOKEN EXPIRY")
	for _, u := range upstreams {
		expiry := u.TokenExpiry.Format("2006-01-02 15:04:05")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", u.Name, u.Type, u.Region, u.Endpoint, expiry)
	}
	w.Flush()
}
