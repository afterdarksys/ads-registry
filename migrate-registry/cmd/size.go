package cmd

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/spf13/cobra"
)

var sizeCmd = &cobra.Command{
	Use:   "size [registry/repository:tag]",
	Short: "Determine the total size of an image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]
		
		fmt.Printf("Fetching image information for %s...\n", ref)
		img, err := crane.Pull(ref, getCraneOptions()...)
		if err != nil {
			return fmt.Errorf("pulling image: %w", err)
		}

		layers, err := img.Layers()
		if err != nil {
			return fmt.Errorf("getting layers: %w", err)
		}

		var totalSize int64
		for _, l := range layers {
			sz, err := l.Size()
			if err != nil {
				return fmt.Errorf("getting layer size: %w", err)
			}
			totalSize += sz
		}

		_, err = img.ConfigName()
		if err == nil {
			rawCfg, err := img.RawConfigFile()
			if err == nil {
				totalSize += int64(len(rawCfg))
			}
		}

		manifestBytes, err := img.RawManifest()
		if err == nil {
			totalSize += int64(len(manifestBytes))
		}

		fmt.Printf("Total size for %s: %s (%d bytes)\n", ref, humanize.Bytes(uint64(totalSize)), totalSize)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sizeCmd)
}
