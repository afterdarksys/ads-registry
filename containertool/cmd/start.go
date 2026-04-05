package cmd

import (
	"context"
	"fmt"

	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [containerID]",
	Short: "Start a stopped container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		fmt.Printf("Starting container %s...\n", id)
		if err := mgr.StartContainer(ctx, id); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}

		fmt.Println("Container started successfully!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
