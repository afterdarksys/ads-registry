package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [image]",
	Short: "Run a container from an image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		imageName := args[0]
		timeout, _ := cmd.Flags().GetInt("timeout")

		ctx := context.Background()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()
		}

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		fmt.Printf("Pulling image %s...\n", imageName)
		out, err := mgr.GetClient().ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}
		defer out.Close()

		// Parse JSON progress stream
		decoder := json.NewDecoder(out)
		layers := make(map[string]string)

		for {
			var msg struct {
				Status         string `json:"status"`
				ID             string `json:"id"`
				Progress       string `json:"progress"`
				ProgressDetail struct {
					Current int64 `json:"current"`
					Total   int64 `json:"total"`
				} `json:"progressDetail"`
			}

			if err := decoder.Decode(&msg); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("error reading pull output: %w", err)
			}

			// Track layer progress
			if msg.ID != "" {
				statusStr := msg.Status
				if msg.Progress != "" {
					statusStr += " " + msg.Progress
				}
				layers[msg.ID] = statusStr

				// Clear screen and reprint (simple version)
				if len(layers) <= 10 { // Only show last 10 layers to avoid spam
					fmt.Printf("\r%-12s: %s", msg.ID, statusStr)
					if !strings.Contains(msg.Status, "Complete") {
						fmt.Print("     ")
					}
					fmt.Println()
				}
			} else if msg.Status != "" {
				// Non-layer status messages (e.g., "Pulling fs layer")
				fmt.Println(msg.Status)
			}
		}

		fmt.Printf("Creating container from %s...\n", imageName)
		resp, err := mgr.GetClient().ContainerCreate(ctx, &container.Config{
			Image: imageName,
			Tty:   true,
		}, nil, nil, nil, "")
		if err != nil {
			return fmt.Errorf("failed to create container: %w", err)
		}

		fmt.Printf("Starting container %s...\n", resp.ID)
		if err := mgr.StartContainer(ctx, resp.ID); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}

		fmt.Printf("Container started successfully! ID: %s\n", resp.ID)
		return nil
	},
}

func init() {
	runCmd.Flags().IntP("timeout", "t", 0, "Timeout in seconds for pull operation (0 = no timeout)")
	rootCmd.AddCommand(runCmd)
}
