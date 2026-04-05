package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var viewCmd = &cobra.Command{
	Use:   "view [containerID]",
	Short: "View details of a container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		details, err := mgr.GetClient().ContainerInspect(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to inspect container: %w", err)
		}

		b, err := json.MarshalIndent(details, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal container details: %w", err)
		}

		fmt.Println(string(b))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(viewCmd)
}
