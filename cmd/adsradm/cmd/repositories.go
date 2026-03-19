package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var reposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Manage repositories",
	Long:  `List repositories and tags`,
}

var listReposCmd = &cobra.Command{
	Use:   "list",
	Short: "List all repositories",
	Run:   runListRepos,
}

var listTagsCmd = &cobra.Command{
	Use:   "tags [repository]",
	Short: "List tags for a repository",
	Args:  cobra.ExactArgs(1),
	Run:   runListTags,
}

var listManifestsCmd = &cobra.Command{
	Use:   "manifests [repository]",
	Short: "List manifests for a repository",
	Args:  cobra.ExactArgs(1),
	Run:   runListManifests,
}

func init() {
	rootCmd.AddCommand(reposCmd)
	reposCmd.AddCommand(listReposCmd)
	reposCmd.AddCommand(listTagsCmd)
	reposCmd.AddCommand(listManifestsCmd)
}

func runListRepos(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/repositories")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var repos []struct {
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.Unmarshal(data, &repos); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPOSITORY\tCREATED")
	for _, r := range repos {
		created := r.CreatedAt.Format("2006-01-02 15:04:05")
		fmt.Fprintf(w, "%s\t%s\n", r.Name, created)
	}
	w.Flush()
}

func runListTags(cmd *cobra.Command, args []string) {
	repo := args[0]
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/repositories/" + repo + "/tags")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var tags []struct {
		Name      string    `json:"name"`
		Digest    string    `json:"digest"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.Unmarshal(data, &tags); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(tags) == 0 {
		fmt.Printf("No tags found for repository: %s\n", repo)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TAG\tDIGEST\tCREATED")
	for _, t := range tags {
		created := t.CreatedAt.Format("2006-01-02 15:04:05")
		digest := t.Digest
		if len(digest) > 19 {
			digest = digest[:19] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, digest, created)
	}
	w.Flush()
}

func runListManifests(cmd *cobra.Command, args []string) {
	repo := args[0]
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/repositories/" + repo + "/manifests")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var manifests []struct {
		Digest    string    `json:"digest"`
		MediaType string    `json:"media_type"`
		Size      int64     `json:"size"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.Unmarshal(data, &manifests); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(manifests) == 0 {
		fmt.Printf("No manifests found for repository: %s\n", repo)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DIGEST\tMEDIA TYPE\tSIZE\tCREATED")
	for _, m := range manifests {
		created := m.CreatedAt.Format("2006-01-02 15:04:05")
		digest := m.Digest
		if len(digest) > 19 {
			digest = digest[:19] + "..."
		}
		mediaType := m.MediaType
		if len(mediaType) > 40 {
			mediaType = mediaType[:37] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", digest, mediaType, m.Size, created)
	}
	w.Flush()
}
