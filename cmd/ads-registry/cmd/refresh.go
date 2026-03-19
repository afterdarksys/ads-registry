package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh authentication token",
	Long:  `Refresh an existing authentication token or get a new one with extended expiration`,
	Run:   runRefresh,
}

func init() {
	rootCmd.AddCommand(refreshCmd)
	refreshCmd.Flags().StringP("username", "u", "", "Username for authentication")
	refreshCmd.Flags().StringP("token", "t", "", "Existing token to refresh (if not provided, will use username/password)")
	refreshCmd.Flags().StringP("registry", "r", "http://localhost:5005", "Registry URL")
	refreshCmd.Flags().StringP("scope", "s", "", "Token scope (e.g., repository:namespace/repo:pull,push)")
}

func runRefresh(cmd *cobra.Command, args []string) {
	registryURL, _ := cmd.Flags().GetString("registry")
	existingToken, _ := cmd.Flags().GetString("token")
	username, _ := cmd.Flags().GetString("username")
	scope, _ := cmd.Flags().GetString("scope")

	var newToken string
	var err error

	if existingToken != "" {
		// Refresh existing token
		newToken, err = refreshExistingToken(registryURL, existingToken)
		if err != nil {
			log.Fatalf("failed to refresh token: %v", err)
		}
	} else {
		// Get new token using username/password
		if username == "" {
			fmt.Print("Username: ")
			fmt.Scanln(&username)
		}

		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("failed to read password: %v", err)
		}
		fmt.Println()

		newToken, err = getNewToken(registryURL, username, string(passwordBytes), scope)
		if err != nil {
			log.Fatalf("failed to get token: %v", err)
		}
	}

	fmt.Println("\nNew Token:")
	fmt.Println(newToken)
	fmt.Println("\nToken saved to: $HOME/.ads-registry-token")

	// Save token to file for convenience
	homeDir, err := os.UserHomeDir()
	if err == nil {
		tokenFile := homeDir + "/.ads-registry-token"
		os.WriteFile(tokenFile, []byte(newToken), 0600)
	}
}

func refreshExistingToken(registryURL, token string) (string, error) {
	req, err := http.NewRequest("POST", registryURL+"/auth/refresh", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("refresh failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	fmt.Printf("Token refreshed successfully (expires in %d seconds)\n", result.ExpiresIn)
	return result.Token, nil
}

func getNewToken(registryURL, username, password, scope string) (string, error) {
	url := registryURL + "/auth/token"
	if scope != "" {
		url += "?scope=" + scope
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Set Basic Auth
	auth := username + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	req.Header.Set("Authorization", "Basic "+encodedAuth)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("authentication failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w\nResponse: %s", err, string(bodyBytes))
	}

	fmt.Printf("Token obtained successfully (expires in %d seconds)\n", result.ExpiresIn)
	return result.Token, nil
}
