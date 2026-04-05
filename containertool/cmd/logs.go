package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs [containerID]",
	Short: "Stream logs from a container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		follow, _ := cmd.Flags().GetBool("follow")
		tail, _ := cmd.Flags().GetString("tail")
		timestamps, _ := cmd.Flags().GetBool("timestamps")
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		opts := container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     follow,
			Timestamps: timestamps,
		}

		if tail != "" {
			opts.Tail = tail
		}

		fmt.Printf("Streaming logs from container %s...\n", id)
		if follow {
			fmt.Println("(Press Ctrl+C to stop)")
		}
		fmt.Println("--------------------------------------------------")

		logs, err := mgr.GetClient().ContainerLogs(ctx, id, opts)
		if err != nil {
			return fmt.Errorf("failed to get container logs: %w", err)
		}
		defer logs.Close()

		if _, err := io.Copy(os.Stdout, logs); err != nil {
			return fmt.Errorf("error reading logs: %w", err)
		}

		return nil
	},
}

func init() {
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output (stream live)")
	logsCmd.Flags().StringP("tail", "n", "all", "Number of lines to show from the end of the logs")
	logsCmd.Flags().BoolP("timestamps", "t", false, "Show timestamps")
	rootCmd.AddCommand(logsCmd)
}
