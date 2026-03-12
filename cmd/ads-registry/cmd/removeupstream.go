package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db/postgres"
	"github.com/spf13/cobra"
)

var removeUpstreamCmd = &cobra.Command{
	Use:   "remove-upstream [name]",
	Short: "Remove an upstream registry",
	Long:  `Remove an upstream container registry configuration.`,
	Args:  cobra.ExactArgs(1),
	Run:   runRemoveUpstream,
}

func init() {
	rootCmd.AddCommand(removeUpstreamCmd)
}

func runRemoveUpstream(cmd *cobra.Command, args []string) {
	name := args[0]

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

	// Get upstream
	upstream, err := store.GetUpstreamByName(context.Background(), name)
	if err != nil {
		log.Fatalf("upstream not found: %v", err)
	}

	// Delete upstream
	if err := store.DeleteUpstream(context.Background(), upstream["id"].(int)); err != nil {
		log.Fatalf("failed to remove upstream: %v", err)
	}

	fmt.Printf("✅ Successfully removed upstream registry: %s\n", name)
}
