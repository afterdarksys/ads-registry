package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		opts := container.ListOptions{
			All: all,
		}

		containers, err := mgr.GetClient().ContainerList(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}

		if len(containers) == 0 {
			fmt.Println("No containers found")
			return nil
		}

		// Print header
		fmt.Printf("%-12s %-20s %-20s %-10s %-15s\n",
			"CONTAINER ID", "IMAGE", "COMMAND", "STATUS", "NAMES")
		fmt.Println(strings.Repeat("-", 80))

		// Print each container
		for _, c := range containers {
			id := c.ID
			if len(id) > 12 {
				id = id[:12]
			}

			image := c.Image
			if len(image) > 20 {
				image = image[:17] + "..."
			}

			command := c.Command
			if len(command) > 20 {
				command = command[:17] + "..."
			}

			status := c.Status
			if len(status) > 10 {
				status = status[:7] + "..."
			}

			names := ""
			if len(c.Names) > 0 {
				names = strings.TrimPrefix(c.Names[0], "/")
				if len(names) > 15 {
					names = names[:12] + "..."
				}
			}

			fmt.Printf("%-12s %-20s %-20s %-10s %-15s\n",
				id, image, command, status, names)
		}

		fmt.Printf("\nTotal: %d container(s)\n", len(containers))
		return nil
	},
}

func init() {
	psCmd.Flags().BoolP("all", "a", false, "Show all containers (default shows just running)")
	rootCmd.AddCommand(psCmd)
}
