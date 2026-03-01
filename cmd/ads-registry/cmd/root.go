package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ads-registry",
	Short: "A high-performance OCI container registry",
	Long: `ads-registry is a fast, statically compiled OCI container registry
designed for edge and enterprise deployments with built-in scanning and RBAC.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
