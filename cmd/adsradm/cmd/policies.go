package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var policiesCmd = &cobra.Command{
	Use:   "policies",
	Short: "Manage security policies",
	Long:  `List and add security policies`,
}

var listPoliciesCmd = &cobra.Command{
	Use:   "list",
	Short: "List all policies",
	Run:   runListPolicies,
}

var addPolicyCmd = &cobra.Command{
	Use:   "add [expression]",
	Short: "Add a new policy",
	Long:  `Add a new policy expression (e.g., "critical_vulns < 5")`,
	Args:  cobra.ExactArgs(1),
	Run:   runAddPolicy,
}

func init() {
	rootCmd.AddCommand(policiesCmd)
	policiesCmd.AddCommand(listPoliciesCmd)
	policiesCmd.AddCommand(addPolicyCmd)
}

func runListPolicies(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/policies")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var policies []map[string]string

	if err := json.Unmarshal(data, &policies); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(policies) == 0 {
		fmt.Println("No policies configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EXPRESSION")
	for _, p := range policies {
		fmt.Fprintf(w, "%s\n", p["expression"])
	}
	w.Flush()
}

func runAddPolicy(cmd *cobra.Command, args []string) {
	expression := args[0]
	client := NewAPIClient()

	payload := map[string]interface{}{
		"expression": expression,
	}

	_, err := client.Post("/api/v1/management/policies", payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Policy added successfully: %s\n", expression)
}
