package cmd

import (
	"context"
	"fmt"

	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var pauseCmd = &cobra.Command{
	Use:   "pause [containerID]",
	Short: "Pause or unpause a running container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		unpause, _ := cmd.Flags().GetBool("unpause")
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		if unpause {
			fmt.Printf("Unpausing container %s...\n", id)
			if err := mgr.UnpauseContainer(ctx, id); err != nil {
				return fmt.Errorf("failed to unpause container: %w", err)
			}
			fmt.Println("Container unpaused successfully!")
		} else {
			fmt.Printf("Pausing container %s...\n", id)
			if err := mgr.PauseContainer(ctx, id); err != nil {
				return fmt.Errorf("failed to pause container: %w", err)
			}
			fmt.Println("Container paused successfully!")
		}

		return nil
	},
}

func init() {
	pauseCmd.Flags().BoolP("unpause", "u", false, "Unpause the container instead of pausing it")
	rootCmd.AddCommand(pauseCmd)
}
