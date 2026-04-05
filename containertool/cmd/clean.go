package cmd

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/filters"
	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up unused Docker resources",
	Long: `Remove unused containers, images, networks, and volumes.
Use --all to also remove unused images, not just dangling ones.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		volumes, _ := cmd.Flags().GetBool("volumes")
		force, _ := cmd.Flags().GetBool("force")
		ctx := context.Background()

		if !force {
			fmt.Println("WARNING: This will remove:")
			fmt.Println("  - All stopped containers")
			fmt.Println("  - All networks not used by at least one container")
			if all {
				fmt.Println("  - All unused images (not just dangling)")
			} else {
				fmt.Println("  - All dangling images")
			}
			if volumes {
				fmt.Println("  - All unused volumes")
			}
			fmt.Print("\nAre you sure you want to continue? [y/N]: ")

			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		fmt.Println("\nCleaning up Docker resources...")

		// Remove stopped containers
		fmt.Print("Removing stopped containers... ")
		containerReport, err := mgr.GetClient().ContainersPrune(ctx, filters.Args{})
		if err != nil {
			return fmt.Errorf("failed to prune containers: %w", err)
		}
		fmt.Printf("Done! (Removed: %d, Space reclaimed: %.2f MB)\n",
			len(containerReport.ContainersDeleted),
			float64(containerReport.SpaceReclaimed)/1024/1024)

		// Remove unused networks
		fmt.Print("Removing unused networks... ")
		networkReport, err := mgr.GetClient().NetworksPrune(ctx, filters.Args{})
		if err != nil {
			return fmt.Errorf("failed to prune networks: %w", err)
		}
		fmt.Printf("Done! (Removed: %d)\n", len(networkReport.NetworksDeleted))

		// Remove dangling/unused images
		fmt.Print("Removing unused images... ")
		imageFilters := filters.NewArgs()
		if !all {
			imageFilters.Add("dangling", "true")
		}
		imageReport, err := mgr.GetClient().ImagesPrune(ctx, imageFilters)
		if err != nil {
			return fmt.Errorf("failed to prune images: %w", err)
		}
		fmt.Printf("Done! (Removed: %d, Space reclaimed: %.2f MB)\n",
			len(imageReport.ImagesDeleted),
			float64(imageReport.SpaceReclaimed)/1024/1024)

		// Remove unused volumes if requested
		if volumes {
			fmt.Print("Removing unused volumes... ")
			volumeReport, err := mgr.GetClient().VolumesPrune(ctx, filters.Args{})
			if err != nil {
				return fmt.Errorf("failed to prune volumes: %w", err)
			}
			fmt.Printf("Done! (Removed: %d, Space reclaimed: %.2f MB)\n",
				len(volumeReport.VolumesDeleted),
				float64(volumeReport.SpaceReclaimed)/1024/1024)
		}

		totalSpace := containerReport.SpaceReclaimed + imageReport.SpaceReclaimed
		if volumes {
			volumeReport, _ := mgr.GetClient().VolumesPrune(ctx, filters.Args{})
			totalSpace += volumeReport.SpaceReclaimed
		}

		fmt.Printf("\nTotal space reclaimed: %.2f MB\n", float64(totalSpace)/1024/1024)
		return nil
	},
}

func init() {
	cleanCmd.Flags().BoolP("all", "a", false, "Remove all unused images, not just dangling ones")
	cleanCmd.Flags().Bool("volumes", false, "Also prune volumes")
	cleanCmd.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	rootCmd.AddCommand(cleanCmd)
}
