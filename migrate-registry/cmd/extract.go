package cmd

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/spf13/cobra"
)

var extractCmd = &cobra.Command{
	Use:   "extract [registry/repository:tag] [output_path.tar]",
	Short: "Extract an image and save it as an OCI tarball",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]
		outFile := args[1]
		
		if !strings.HasSuffix(outFile, ".tar") {
			outFile += ".tar"
		}

		fmt.Printf("Pulling image %s...\n", ref)
		img, err := crane.Pull(ref, getCraneOptions()...)
		if err != nil {
			return fmt.Errorf("pulling image %s: %w", ref, err)
		}

		fmt.Printf("Saving image to %s...\n", outFile)
		if err := crane.Save(img, ref, outFile); err != nil {
			return fmt.Errorf("saving image: %w", err)
		}

		fmt.Println("Extraction complete.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(extractCmd)
}
