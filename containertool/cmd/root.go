package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "containertool",
	Short: "A registry thin client & container runner for ads-registry",
	Long: `containertool serves as an independent, lightweight reference client 
for pulling images from ads-registry and managing containers via the Docker API.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func init() {
	// Root command flags if needed
}
