package cmd

import (
	"context"
	"fmt"

	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [containerID]",
	Short: "Stop a running container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		fmt.Printf("Stopping container %s...\n", id)
		if err := mgr.StopContainer(ctx, id); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}

		fmt.Println("Container stopped successfully!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
