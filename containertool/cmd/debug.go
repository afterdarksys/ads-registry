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

var debugCmd = &cobra.Command{
	Use:   "debug [containerID]",
	Short: "Troubleshoot a container that failed to start",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		interactive, _ := cmd.Flags().GetBool("interactive")
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		// Inspect the container
		fmt.Printf("Analyzing failure for container %s...\n", id)
		details, err := mgr.GetClient().ContainerInspect(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to inspect container: %w", err)
		}

		fmt.Printf("Status: %s\n", details.State.Status)
		fmt.Printf("Exit Code: %d\n", details.State.ExitCode)
		if details.State.OOMKilled {
			fmt.Println("Error: Container was OOMKilled! (Out of Memory)")
		}
		if details.State.Error != "" {
			fmt.Printf("Daemon Error: %s\n", details.State.Error)
		}

		// Pull the last few lines of logs
		fmt.Println("\n--- Last output before exit ---")
		out, err := mgr.GetClient().ContainerLogs(ctx, id, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "50",
		})
		if err == nil {
			io.Copy(os.Stdout, out)
			out.Close()
		} else {
			fmt.Printf("Could not retrieve logs: %v\n", err)
		}
		fmt.Println("-------------------------------")

		if interactive {
			fmt.Println("\n[Interactive Debug] Launching companion debug container with /bin/sh entrypoint...")
			resp, err := mgr.GetClient().ContainerCreate(ctx, &container.Config{
				Image:      details.Image,
				Entrypoint: []string{"/bin/sh"},
				Tty:        true,
				OpenStdin:  true,
			}, nil, nil, nil, "")
			
			if err != nil {
				return fmt.Errorf("failed to create debug container: %w", err)
			}
			
			if err := mgr.StartContainer(ctx, resp.ID); err != nil {
				return fmt.Errorf("failed to start debug container: %w", err)
			}
			
			fmt.Printf("Debug container started successfully (ID: %s)!\n", resp.ID)
			fmt.Printf("You can attach to it via standard docker CLI: docker exec -it %s /bin/sh (if docker daemon is up)\n", resp.ID)
		} else {
			fmt.Println("\nTip: Use `--interactive` to automatically spin up a shell entrypoint test container.")
		}

		return nil
	},
}

func init() {
	debugCmd.Flags().BoolP("interactive", "i", false, "Start an interactive fallback /bin/sh container from the same image")
	rootCmd.AddCommand(debugCmd)
}
