package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db/postgres"
	"github.com/spf13/cobra"
)

var listUpstreamsCmd = &cobra.Command{
	Use:   "list-upstreams",
	Short: "List all configured upstream registries",
	Long:  `List all configured upstream container registries with their status and details.`,
	Run:   runListUpstreams,
}

func init() {
	rootCmd.AddCommand(listUpstreamsCmd)
}

func runListUpstreams(cmd *cobra.Command, args []string) {
	// Load config
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "config.json"
	}
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize database (upstream registries require PostgreSQL)
	if cfg.Database.Driver != "postgres" && cfg.Database.Driver != "pgsqllite" {
		log.Fatal("Upstream registries require PostgreSQL database - SQLite is not supported for this feature")
	}

	store, err := postgres.New(cfg.Database)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer store.Close()

	// List upstreams
	upstreams, err := store.ListUpstreams(context.Background())
	if err != nil {
		log.Fatalf("failed to list upstreams: %v", err)
	}

	if len(upstreams) == 0 {
		fmt.Println("No upstream registries configured.")
		fmt.Println("\nAdd one with: ads-registry add-upstream --help")
		return
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tENDPOINT\tREGION\tSTATUS\tCACHE\tLAST REFRESH")
	fmt.Fprintln(w, "----\t----\t--------\t------\t------\t-----\t------------")

	for _, u := range upstreams {
		status := "✅ enabled"
		if !u["enabled"].(bool) {
			status = "❌ disabled"
		}

		cache := "enabled"
		if !u["cache_enabled"].(bool) {
			cache = "disabled"
		}

		lastRefresh := "never"
		if lr, ok := u["last_refresh"].(string); ok && lr != "" {
			lastRefresh = lr
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			u["name"],
			u["type"],
			u["endpoint"],
			u["region"],
			status,
			cache,
			lastRefresh,
		)
	}

	w.Flush()

	fmt.Printf("\nTotal: %d upstream(s)\n", len(upstreams))
	fmt.Println("\nUsage: docker pull localhost:5005/<upstream-name>/<repository>:<tag>")
}
