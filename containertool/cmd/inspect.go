package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [image]",
	Short: "Inspect image layers, size, and metadata",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		imageName := args[0]
		verbose, _ := cmd.Flags().GetBool("verbose")
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		fmt.Printf("Inspecting image: %s\n", imageName)
		fmt.Println("--------------------------------------------------")

		// Get image details
		inspect, _, err := mgr.GetClient().ImageInspectWithRaw(ctx, imageName)
		if err != nil {
			return fmt.Errorf("failed to inspect image: %w", err)
		}

		// Display basic info
		fmt.Printf("ID: %s\n", inspect.ID)
		fmt.Printf("Created: %s\n", inspect.Created)
		fmt.Printf("Size: %.2f MB\n", float64(inspect.Size)/1024/1024)
		fmt.Printf("Architecture: %s\n", inspect.Architecture)
		fmt.Printf("OS: %s\n", inspect.Os)

		if inspect.Config != nil {
			fmt.Println("\nConfiguration:")
			if len(inspect.Config.Env) > 0 {
				fmt.Printf("  Environment Variables: %d\n", len(inspect.Config.Env))
				if verbose {
					for _, env := range inspect.Config.Env {
						fmt.Printf("    - %s\n", env)
					}
				}
			}
			if len(inspect.Config.Cmd) > 0 {
				fmt.Printf("  Command: %v\n", inspect.Config.Cmd)
			}
			if len(inspect.Config.Entrypoint) > 0 {
				fmt.Printf("  Entrypoint: %v\n", inspect.Config.Entrypoint)
			}
			if len(inspect.Config.ExposedPorts) > 0 {
				fmt.Printf("  Exposed Ports: %d\n", len(inspect.Config.ExposedPorts))
				if verbose {
					for port := range inspect.Config.ExposedPorts {
						fmt.Printf("    - %s\n", port)
					}
				}
			}
		}

		// Display layer info
		fmt.Println("\nLayers:")
		fmt.Printf("  Total Layers: %d\n", len(inspect.RootFS.Layers))
		if verbose {
			for i, layer := range inspect.RootFS.Layers {
				fmt.Printf("  [%d] %s\n", i+1, layer)
			}
		}

		// Note: History field removed in newer Docker API versions
		// Image layer information is sufficient for most use cases

		// Check for potential security issues
		fmt.Println("\nSecurity Analysis:")
		if inspect.Config != nil && inspect.Config.User == "" {
			fmt.Println("  WARNING: Image runs as root (no USER specified)")
		} else if inspect.Config != nil {
			fmt.Printf("  OK: Image runs as user '%s'\n", inspect.Config.User)
		}

		// Display full JSON if requested
		fullJson, _ := cmd.Flags().GetBool("json")
		if fullJson {
			fmt.Println("\n--------------------------------------------------")
			fmt.Println("Full JSON Output:")
			b, err := json.MarshalIndent(inspect, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal JSON: %w", err)
			}
			fmt.Println(string(b))
		}

		fmt.Println("--------------------------------------------------")
		return nil
	},
}

func init() {
	inspectCmd.Flags().BoolP("verbose", "v", false, "Show detailed layer and history information")
	inspectCmd.Flags().Bool("json", false, "Output full JSON representation")
	rootCmd.AddCommand(inspectCmd)
}
