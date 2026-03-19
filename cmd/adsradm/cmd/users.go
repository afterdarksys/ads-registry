package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage registry users",
	Long:  `Create, list, update, and delete registry users`,
}

var listUsersCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	Run:   runListUsers,
}

var createUserCmd = &cobra.Command{
	Use:   "create [username]",
	Short: "Create a new user",
	Args:  cobra.ExactArgs(1),
	Run:   runCreateUser,
}

var deleteUserCmd = &cobra.Command{
	Use:   "delete [username]",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	Run:   runDeleteUser,
}

var updateUserCmd = &cobra.Command{
	Use:   "update [username]",
	Short: "Update user scopes",
	Args:  cobra.ExactArgs(1),
	Run:   runUpdateUser,
}

var resetPasswordCmd = &cobra.Command{
	Use:   "reset-password [username]",
	Short: "Reset user password",
	Args:  cobra.ExactArgs(1),
	Run:   runResetPassword,
}

var userScopes []string

func init() {
	rootCmd.AddCommand(usersCmd)
	usersCmd.AddCommand(listUsersCmd)
	usersCmd.AddCommand(createUserCmd)
	usersCmd.AddCommand(deleteUserCmd)
	usersCmd.AddCommand(updateUserCmd)
	usersCmd.AddCommand(resetPasswordCmd)

	createUserCmd.Flags().StringSliceVarP(&userScopes, "scopes", "s", []string{"*"}, "User scopes (comma-separated)")
	updateUserCmd.Flags().StringSliceVarP(&userScopes, "scopes", "s", []string{}, "User scopes (comma-separated)")
	updateUserCmd.MarkFlagRequired("scopes")
}

func runListUsers(cmd *cobra.Command, args []string) {
	client := NewAPIClient()

	data, err := client.Get("/api/v1/management/users")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var users []struct {
		Username string   `json:"username"`
		Scopes   []string `json:"scopes"`
	}

	if err := json.Unmarshal(data, &users); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USERNAME\tSCOPES")
	for _, user := range users {
		scopes := ""
		for i, s := range user.Scopes {
			if i > 0 {
				scopes += ", "
			}
			scopes += s
		}
		fmt.Fprintf(w, "%s\t%s\n", user.Username, scopes)
	}
	w.Flush()
}

func runCreateUser(cmd *cobra.Command, args []string) {
	username := args[0]

	fmt.Print("Enter password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	if string(passwordBytes) != string(confirmBytes) {
		fmt.Fprintln(os.Stderr, "Error: passwords do not match")
		os.Exit(1)
	}

	password := string(passwordBytes)
	if len(password) < 8 {
		fmt.Fprintln(os.Stderr, "Error: password must be at least 8 characters long")
		os.Exit(1)
	}

	client := NewAPIClient()

	payload := map[string]interface{}{
		"username": username,
		"password": password,
		"scopes":   userScopes,
	}

	_, err = client.Post("/api/v1/management/users", payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User '%s' created successfully with scopes: %v\n", username, userScopes)
}

func runDeleteUser(cmd *cobra.Command, args []string) {
	username := args[0]

	client := NewAPIClient()

	_, err := client.Delete("/api/v1/management/users/" + username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User '%s' deleted successfully\n", username)
}

func runUpdateUser(cmd *cobra.Command, args []string) {
	username := args[0]

	client := NewAPIClient()

	payload := map[string]interface{}{
		"scopes": userScopes,
	}

	_, err := client.Put("/api/v1/management/users/"+username, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User '%s' updated successfully with scopes: %v\n", username, userScopes)
}

func runResetPassword(cmd *cobra.Command, args []string) {
	username := args[0]

	fmt.Print("Enter new password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	fmt.Print("Confirm new password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	if string(passwordBytes) != string(confirmBytes) {
		fmt.Fprintln(os.Stderr, "Error: passwords do not match")
		os.Exit(1)
	}

	password := string(passwordBytes)
	if len(password) < 8 {
		fmt.Fprintln(os.Stderr, "Error: password must be at least 8 characters long")
		os.Exit(1)
	}

	client := NewAPIClient()

	payload := map[string]interface{}{
		"password": password,
	}

	_, err = client.Post("/api/v1/management/users/"+username+"/reset-password", payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Password for user '%s' reset successfully\n", username)
}
