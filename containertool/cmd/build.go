package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/ryan/ads-registry/containertool/pkg/runtime"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build [path] [image-tag]",
	Short: "Build a docker image from a directory",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		buildContext := args[0]
		imageTag := args[1]
		ctx := context.Background()

		mgr, err := runtime.NewManager()
		if err != nil {
			return err
		}
		defer mgr.Close()

		fmt.Printf("Packaging build context from %s...\n", buildContext)
		buf := new(bytes.Buffer)
		if err := runtime.PackBuildContext(buildContext, buf); err != nil {
			return fmt.Errorf("failed to package build context: %w", err)
		}

		fmt.Printf("Building image %s...\n", imageTag)
		opt := types.ImageBuildOptions{
			Tags:       []string{imageTag},
			Dockerfile: "Dockerfile",
			Remove:     true,
		}

		res, err := mgr.GetClient().ImageBuild(ctx, buf, opt)
		if err != nil {
			return fmt.Errorf("failed to build image: %w", err)
		}
		defer res.Body.Close()

		// Docker build output is JSON stream
		decoder := json.NewDecoder(res.Body)
		for {
			var msg map[string]interface{}
			if err := decoder.Decode(&msg); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("error reading build output: %w", err)
			}
			if stream, ok := msg["stream"]; ok {
				fmt.Print(stream)
			}
			if errDetail, ok := msg["errorDetail"]; ok {
				return fmt.Errorf("build failed: %v", errDetail)
			}
		}

		fmt.Println("Image built successfully!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
}
