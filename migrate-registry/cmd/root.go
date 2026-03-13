package cmd

import (
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/spf13/cobra"
)

var (
	insecure bool
	verbose  bool
)

var rootCmd = &cobra.Command{
	Use:   "migrate-registry",
	Short: "A toolkit for migrating and inspecting OCI container registries",
	Long:  `migrate-registry is a CLI tool built to inspect, extract, and migrate items between OCI registries such as OCIR, ECR, ACR, Artifactory, and more.`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "Allow connections to SSL sites without valid certificates")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
}

// getCraneOptions returns the shared crane options
func getCraneOptions() []crane.Option {
	var opts []crane.Option
	if insecure {
		opts = append(opts, crane.Insecure)
	}
	// auth is automatically handled by crane from docker config unless overridden
	return opts
}
