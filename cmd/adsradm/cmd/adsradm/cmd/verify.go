package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Run verification tests against the registry",
	Long:  `Run a comprehensive test suite to verify registry functionality`,
}

var verifyAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all verification tests",
	Run:   runVerifyAll,
}

var verifyHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Verify registry health and connectivity",
	Run:   runVerifyHealth,
}

var verifyAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Verify authentication and admin access",
	Run:   runVerifyAuth,
}

var verifyReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Verify repository operations (list, tags, manifests)",
	Run:   runVerifyRepos,
}

var verifyMultiLevelCmd = &cobra.Command{
	Use:   "multi-level",
	Short: "Verify multi-level repository support",
	Run:   runVerifyMultiLevel,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
	verifyCmd.AddCommand(verifyAllCmd)
	verifyCmd.AddCommand(verifyHealthCmd)
	verifyCmd.AddCommand(verifyAuthCmd)
	verifyCmd.AddCommand(verifyReposCmd)
	verifyCmd.AddCommand(verifyMultiLevelCmd)
}

type TestResult struct {
	Name    string
	Passed  bool
	Message string
	Error   error
}

func printResults(results []TestResult) {
	passed := 0
	failed := 0

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("VERIFICATION RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, result := range results {
		status := "✅ PASS"
		if !result.Passed {
			status = "❌ FAIL"
			failed++
		} else {
			passed++
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", status, result.Name, result.Message)
		if result.Error != nil {
			fmt.Fprintf(w, "\t\tError: %v\n", result.Error)
		}
	}
	w.Flush()

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total: %d | Passed: %d | Failed: %d\n", len(results), passed, failed)
	fmt.Println(strings.Repeat("=", 60))

	if failed > 0 {
		os.Exit(1)
	}
}

func runVerifyAll(cmd *cobra.Command, args []string) {
	fmt.Println("🔍 Running comprehensive registry verification suite...")
	fmt.Printf("Registry: %s\n", getAPIURL())
	fmt.Printf("Timestamp: %s\n\n", time.Now().Format(time.RFC3339))

	var results []TestResult

	// Health checks
	results = append(results, verifyHealth()...)

	// Auth checks
	results = append(results, verifyAuth()...)

	// Repository operations
	results = append(results, verifyRepositoryOps()...)

	// Multi-level repository support
	results = append(results, verifyMultiLevelRepos()...)

	printResults(results)
}

func runVerifyHealth(cmd *cobra.Command, args []string) {
	results := verifyHealth()
	printResults(results)
}

func runVerifyAuth(cmd *cobra.Command, args []string) {
	results := verifyAuth()
	printResults(results)
}

func runVerifyRepos(cmd *cobra.Command, args []string) {
	results := verifyRepositoryOps()
	printResults(results)
}

func runVerifyMultiLevel(cmd *cobra.Command, args []string) {
	results := verifyMultiLevelRepos()
	printResults(results)
}

func verifyHealth() []TestResult {
	var results []TestResult
	client := NewAPIClient()

	// Test 1: API connectivity
	result := TestResult{Name: "API Connectivity"}
	_, err := client.Get("/api/v1/management/stats")
	if err != nil {
		result.Passed = false
		result.Message = "Cannot connect to management API"
		result.Error = err
	} else {
		result.Passed = true
		result.Message = "Management API accessible"
	}
	results = append(results, result)

	// Test 2: Get stats
	result = TestResult{Name: "Registry Statistics"}
	data, err := client.Get("/api/v1/management/stats")
	if err != nil {
		result.Passed = false
		result.Message = "Failed to retrieve stats"
		result.Error = err
	} else {
		var stats map[string]interface{}
		if err := json.Unmarshal(data, &stats); err == nil {
			result.Passed = true
			result.Message = fmt.Sprintf("Repos: %v, Storage: %v", stats["total_repos"], stats["storage_used"])
		} else {
			result.Passed = false
			result.Message = "Invalid stats response"
			result.Error = err
		}
	}
	results = append(results, result)

	return results
}

func verifyAuth() []TestResult {
	var results []TestResult
	client := NewAPIClient()

	// Test 1: Admin authentication
	result := TestResult{Name: "Admin Authentication"}
	_, err := client.Get("/api/v1/management/users")
	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
			result.Passed = false
			result.Message = "Authentication failed - invalid token"
			result.Error = err
		} else if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "Forbidden") {
			result.Passed = false
			result.Message = "Authentication succeeded but lacks admin privileges"
			result.Error = err
		} else {
			result.Passed = false
			result.Message = "Request failed"
			result.Error = err
		}
	} else {
		result.Passed = true
		result.Message = "Admin credentials valid"
	}
	results = append(results, result)

	// Test 2: User management access
	result = TestResult{Name: "User Management Access"}
	data, err := client.Get("/api/v1/management/users")
	if err != nil {
		result.Passed = false
		result.Message = "Cannot access user management"
		result.Error = err
	} else {
		var users []interface{}
		if err := json.Unmarshal(data, &users); err == nil {
			result.Passed = true
			result.Message = fmt.Sprintf("Access granted - %d users", len(users))
		} else {
			result.Passed = false
			result.Message = "Invalid response format"
			result.Error = err
		}
	}
	results = append(results, result)

	return results
}

func verifyRepositoryOps() []TestResult {
	var results []TestResult
	client := NewAPIClient()

	// Test 1: List repositories
	result := TestResult{Name: "List Repositories"}
	data, err := client.Get("/api/v1/management/repositories")
	if err != nil {
		result.Passed = false
		result.Message = "Failed to list repositories"
		result.Error = err
	} else {
		var repos []string
		if err := json.Unmarshal(data, &repos); err == nil {
			result.Passed = true
			result.Message = fmt.Sprintf("Found %d repositories", len(repos))
		} else {
			result.Passed = false
			result.Message = "Invalid repository list response"
			result.Error = err
		}
	}
	results = append(results, result)

	// Test 2: Get tags for first single-level repo
	result = TestResult{Name: "Single-Level Repo Tags"}
	data, err = client.Get("/api/v1/management/repositories")
	if err == nil {
		var repos []string
		if err := json.Unmarshal(data, &repos); err == nil && len(repos) > 0 {
			// Find first single-level repo (no slash)
			var singleRepo string
			for _, repo := range repos {
				if !strings.Contains(repo, "/") {
					singleRepo = repo
					break
				}
			}

			if singleRepo != "" {
				tagData, err := client.Get("/api/v1/management/repositories/" + singleRepo + "/tags")
				if err != nil {
					result.Passed = false
					result.Message = fmt.Sprintf("Failed to get tags for %s", singleRepo)
					result.Error = err
				} else {
					var tags []interface{}
					if err := json.Unmarshal(tagData, &tags); err == nil {
						result.Passed = true
						result.Message = fmt.Sprintf("%s has %d tags", singleRepo, len(tags))
					} else {
						result.Passed = false
						result.Message = "Invalid tags response"
						result.Error = err
					}
				}
			} else {
				result.Passed = true
				result.Message = "No single-level repos to test (skipped)"
			}
		}
	} else {
		result.Passed = false
		result.Message = "Cannot list repos for tag test"
		result.Error = err
	}
	results = append(results, result)

	return results
}

func verifyMultiLevelRepos() []TestResult {
	var results []TestResult
	client := NewAPIClient()

	// Test 1: Find multi-level repository
	result := TestResult{Name: "Multi-Level Repo Detection"}
	data, err := client.Get("/api/v1/management/repositories")
	var multiLevelRepo string
	if err != nil {
		result.Passed = false
		result.Message = "Failed to list repositories"
		result.Error = err
	} else {
		var repos []string
		if err := json.Unmarshal(data, &repos); err == nil {
			for _, repo := range repos {
				if strings.Contains(repo, "/") {
					multiLevelRepo = repo
					break
				}
			}
			if multiLevelRepo != "" {
				result.Passed = true
				result.Message = fmt.Sprintf("Found multi-level repo: %s", multiLevelRepo)
			} else {
				result.Passed = true
				result.Message = "No multi-level repos found (skipped)"
			}
		} else {
			result.Passed = false
			result.Message = "Invalid response format"
			result.Error = err
		}
	}
	results = append(results, result)

	// Test 2: Get tags for multi-level repo
	if multiLevelRepo != "" {
		result = TestResult{Name: "Multi-Level Repo Tags"}
		tagData, err := client.Get("/api/v1/management/repositories/" + multiLevelRepo + "/tags")
		if err != nil {
			result.Passed = false
			result.Message = fmt.Sprintf("Failed to get tags for %s", multiLevelRepo)
			result.Error = err
		} else {
			var tags []interface{}
			if err := json.Unmarshal(tagData, &tags); err == nil {
				result.Passed = true
				result.Message = fmt.Sprintf("%s has %d tags", multiLevelRepo, len(tags))
			} else {
				result.Passed = false
				result.Message = "Invalid tags response"
				result.Error = err
			}
		}
		results = append(results, result)

		// Test 3: Get manifests for multi-level repo
		result = TestResult{Name: "Multi-Level Repo Manifests"}
		manifestData, err := client.Get("/api/v1/management/repositories/" + multiLevelRepo + "/manifests")
		if err != nil {
			result.Passed = false
			result.Message = fmt.Sprintf("Failed to get manifests for %s", multiLevelRepo)
			result.Error = err
		} else {
			var manifests []interface{}
			if err := json.Unmarshal(manifestData, &manifests); err == nil {
				result.Passed = true
				result.Message = fmt.Sprintf("%s has %d manifests", multiLevelRepo, len(manifests))
			} else {
				result.Passed = false
				result.Message = "Invalid manifests response"
				result.Error = err
			}
		}
		results = append(results, result)
	}

	return results
}
