package cmd

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/spf13/cobra"
)

var countCmd = &cobra.Command{
	Use:   "count [registry/repository]",
	Short: "Count the number of tags in a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoName := args[0]
		
		fmt.Printf("Fetching tags for %s...\n", repoName)
		tags, err := crane.ListTags(repoName, getCraneOptions()...)
		if err != nil {
			return fmt.Errorf("listing tags: %w", err)
		}

		fmt.Printf("Repository %s contains %d tags.\n", repoName, len(tags))
		if verbose {
			for _, t := range tags {
				fmt.Printf("- %s\n", t)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(countCmd)
}
