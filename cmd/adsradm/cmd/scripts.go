package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var scriptsCmd = &cobra.Command{
	Use:   "scripts",
	Short: "Manage automation scripts",
	Long:  `List, view, upload, and delete Starlark automation scripts`,
}

var listScriptsCmd = &cobra.Command{
	Use:   "list",
	Short: "List all scripts",
	Run:   runListScripts,
}

var getScriptCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Get script content",
	Args:  cobra.ExactArgs(1),
	Run:   runGetScript,
}

var uploadScriptCmd = &cobra.Command{
	Use:   "upload [name] [file]",
	Short: "Upload a script from file",
	Args:  cobra.ExactArgs(2),
	Run:   runUploadScript,
}

var deleteScriptCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a script",
	Args:  cobra.ExactArgs(1),
	Run:   runDeleteScript,
}

func init() {
	rootCmd.AddCommand(scriptsCmd)
	scriptsCmd.AddCommand(listScriptsCmd)
	scriptsCmd.AddCommand(getScriptCmd)
	scriptsCmd.AddCommand(uploadScriptCmd)
	scriptsCmd.AddCommand(deleteScriptCmd)
}

func runListScripts(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/scripts")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var scripts []string

	if err := json.Unmarshal(data, &scripts); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(scripts) == 0 {
		fmt.Println("No scripts found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SCRIPT")
	for _, s := range scripts {
		fmt.Fprintf(w, "%s\n", s)
	}
	w.Flush()
}

func runGetScript(cmd *cobra.Command, args []string) {
	name := args[0]
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/scripts/" + name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(string(data))
}

func runUploadScript(cmd *cobra.Command, args []string) {
	name := args[0]
	filePath := args[1]

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	client := NewAPIClient()

	// Upload the script content using PUT
	_, err = client.Put("/api/v1/management/scripts/"+name, string(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Script '%s' uploaded successfully (%d bytes)\n", name, len(content))
}

func runDeleteScript(cmd *cobra.Command, args []string) {
	name := args[0]
	client := NewAPIClient()

	_, err := client.Delete("/api/v1/management/scripts/" + name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Script '%s' deleted successfully\n", name)
}
