package cmd

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate [source_registry/repository:tag] [destination_registry/repository:tag]",
	Short: "Migrate an image from a source to a destination registry",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		src := args[0]
		dst := args[1]
		
		fmt.Printf("Migrating %s to %s...\n", src, dst)
		
		if err := crane.Copy(src, dst, getCraneOptions()...); err != nil {
			return fmt.Errorf("migrating image: %w", err)
		}

		fmt.Println("Migration complete.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
