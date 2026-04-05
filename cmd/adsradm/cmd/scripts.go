package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	Use:     "get [name]",
	Aliases: []string{"view"},
	Short:   "Get script content",
	Args:    cobra.ExactArgs(1),
	Run:     runGetScript,
}

var uploadScriptCmd = &cobra.Command{
	Use:     "upload [name] [file]",
	Aliases: []string{"replace"},
	Short:   "Upload a script from file",
	Args:    cobra.ExactArgs(2),
	Run:     runUploadScript,
}

var deleteScriptCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a script",
	Args:  cobra.ExactArgs(1),
	Run:   runDeleteScript,
}

var enableScriptCmd = &cobra.Command{
	Use:   "enable [name]",
	Short: "Enable a script",
	Args:  cobra.ExactArgs(1),
	Run:   runEnableScript,
}

var disableScriptCmd = &cobra.Command{
	Use:   "disable [name]",
	Short: "Disable a script",
	Args:  cobra.ExactArgs(1),
	Run:   runDisableScript,
}

var editScriptCmd = &cobra.Command{
	Use:   "edit [name]",
	Short: "Edit a script interactively",
	Args:  cobra.ExactArgs(1),
	Run:   runEditScript,
}

func init() {
	rootCmd.AddCommand(scriptsCmd)
	scriptsCmd.AddCommand(listScriptsCmd)
	scriptsCmd.AddCommand(getScriptCmd)
	scriptsCmd.AddCommand(uploadScriptCmd)
	scriptsCmd.AddCommand(deleteScriptCmd)
	scriptsCmd.AddCommand(enableScriptCmd)
	scriptsCmd.AddCommand(disableScriptCmd)
	scriptsCmd.AddCommand(editScriptCmd)
}

func runListScripts(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/scripts")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var scripts []struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}

	if err := json.Unmarshal(data, &scripts); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(scripts) == 0 {
		fmt.Println("No scripts found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SCRIPT\tSTATUS")
	for _, s := range scripts {
		status := "ENABLED"
		if !s.Enabled {
			status = "DISABLED"
		}
		fmt.Fprintf(w, "%s\t%s\n", s.Name, status)
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

func runEnableScript(cmd *cobra.Command, args []string) {
	name := args[0]
	client := NewAPIClient()

	_, err := client.Post(fmt.Sprintf("/api/v1/management/scripts/%s/enable", name), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Script '%s' enabled successfully\n", name)
}

func runDisableScript(cmd *cobra.Command, args []string) {
	name := args[0]
	client := NewAPIClient()

	_, err := client.Post(fmt.Sprintf("/api/v1/management/scripts/%s/disable", name), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Script '%s' disabled successfully\n", name)
}

func runEditScript(cmd *cobra.Command, args []string) {
	name := args[0]
	client := NewAPIClient()

	// 1. Fetch current content
	data, err := client.Get("/api/v1/management/scripts/" + name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching script %s: %v\n", name, err)
		os.Exit(1)
	}

	// 2. Create temp file
	tmpFile, err := os.CreateTemp("", "adsradm-script-*.star")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp file: %v\n", err)
		os.Exit(1)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		fmt.Fprintf(os.Stderr, "Error writing to temp file: %v\n", err)
		os.Exit(1)
	}
	tmpFile.Close()

	// 3. Open in editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	execCmd := exec.Command("sh", "-c", editor+" "+tmpName)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Editor error: %v\n", err)
		os.Exit(1)
	}

	// 4. Read modified content
	modifiedData, err := os.ReadFile(tmpName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading modified file: %v\n", err)
		os.Exit(1)
	}

	if string(modifiedData) == string(data) {
		fmt.Println("No changes made.")
		return
	}

	// 5. Upload modifications
	_, err = client.Put("/api/v1/management/scripts/"+name, string(modifiedData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error saving script: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Script '%s' updated successfully\n", name)
}
