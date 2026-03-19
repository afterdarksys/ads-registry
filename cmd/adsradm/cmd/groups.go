package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var groupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "Manage user groups",
	Long:  `Create groups and manage group membership`,
}

var listGroupsCmd = &cobra.Command{
	Use:   "list",
	Short: "List all groups",
	Run:   runListGroups,
}

var createGroupCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new group",
	Args:  cobra.ExactArgs(1),
	Run:   runCreateGroup,
}

var addUserToGroupCmd = &cobra.Command{
	Use:   "add-user [group] [username]",
	Short: "Add a user to a group",
	Args:  cobra.ExactArgs(2),
	Run:   runAddUserToGroup,
}

func init() {
	rootCmd.AddCommand(groupsCmd)
	groupsCmd.AddCommand(listGroupsCmd)
	groupsCmd.AddCommand(createGroupCmd)
	groupsCmd.AddCommand(addUserToGroupCmd)
}

func runListGroups(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/groups")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var groups []struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(data, &groups); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(groups) == 0 {
		fmt.Println("No groups found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "GROUP")
	for _, g := range groups {
		fmt.Fprintf(w, "%s\n", g.Name)
	}
	w.Flush()
}

func runCreateGroup(cmd *cobra.Command, args []string) {
	name := args[0]
	client := NewAPIClient()

	payload := map[string]interface{}{
		"name": name,
	}

	_, err := client.Post("/api/v1/management/groups", payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Group '%s' created successfully\n", name)
}

func runAddUserToGroup(cmd *cobra.Command, args []string) {
	group := args[0]
	username := args[1]
	client := NewAPIClient()

	payload := map[string]interface{}{
		"username": username,
	}

	_, err := client.Post("/api/v1/management/groups/"+group+"/users", payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User '%s' added to group '%s' successfully\n", username, group)
}
