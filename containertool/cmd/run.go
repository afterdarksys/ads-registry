package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

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
		ctx := context.Background()

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
		if _, err := io.Copy(os.Stdout, out); err != nil {
			return fmt.Errorf("error reading pull output: %w", err)
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
	rootCmd.AddCommand(runCmd)
}
