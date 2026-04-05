package cmd

import (
	"context"
	"fmt"
	"log"
	"syscall"

	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db/postgres"
	"github.com/ryan/ads-registry/internal/db/sqlite"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var createUserCmd = &cobra.Command{
	Use:   "create-user [username]",
	Short: "Create a new registry user",
	Long:  `Create a new user with username and password for registry authentication`,
	Args:  cobra.ExactArgs(1),
	Run:   runCreateUser,
}

func init() {
	rootCmd.AddCommand(createUserCmd)
	createUserCmd.Flags().StringSliceP("scopes", "s", []string{"*"}, "User scopes (comma-separated)")
	createUserCmd.Flags().StringP("password", "p", "", "User password (optional, skips prompt)")
}

func runCreateUser(cmd *cobra.Command, args []string) {
	username := args[0]
	scopes, _ := cmd.Flags().GetStringSlice("scopes")

	// Load config
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "config.json"
	}
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	password, _ := cmd.Flags().GetString("password")
	if password == "" {
		// Prompt for password
		fmt.Print("Enter password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("failed to read password: %v", err)
		}
		fmt.Println()

		fmt.Print("Confirm password: ")
		confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("failed to read password: %v", err)
		}
		fmt.Println()

		if string(passwordBytes) != string(confirmBytes) {
			log.Fatal("passwords do not match")
		}
		password = string(passwordBytes)
	}
	if len(password) < 8 {
		log.Fatal("password must be at least 8 characters long")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}

	// Initialize database
	var store interface {
		CreateUser(ctx context.Context, username, passwordHash string, scopes []string) error
		Close() error
	}

	switch cfg.Database.Driver {
	case "postgres":
		store, err = postgres.New(cfg.Database)
		if err != nil {
			log.Printf("Falling back to SQLite for user creation due to Postgres error: %v", err)
			cfg.Database.Driver = "sqlite3"
			cfg.Database.DSN = "data/registry.db?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&cache=shared"
			store, err = sqlite.New(cfg.Database)
		}
	case "sqlite3":
		store, err = sqlite.New(cfg.Database)
	default:
		log.Fatalf("unsupported database driver: %s", cfg.Database.Driver)
	}

	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer store.Close()

	// Create user
	ctx := context.Background()
	if err := store.CreateUser(ctx, username, string(hashedPassword), scopes); err != nil {
		log.Fatalf("failed to create user: %v", err)
	}

	fmt.Printf("User '%s' created successfully with scopes: %v\n", username, scopes)
}
