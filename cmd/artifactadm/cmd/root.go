package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile     string
	registryURL string
	authToken   string
	format      string
	namespace   string
	verbose     bool
)

var rootCmd = &cobra.Command{
	Use:   "artifactadm",
	Short: "Universal artifact registry management tool",
	Long: `artifactadm is a comprehensive CLI for managing multi-format artifacts in ADS Registry.

Supports: npm, pypi, helm, go, apt, composer, cocoapods, brew

Features:
  - Package publishing and management
  - Security scanning and verification
  - Repository maintenance and mirroring
  - Cross-format operations`,
	Version: "1.0.0",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.artifactadm.yaml)")
	rootCmd.PersistentFlags().StringVar(&registryURL, "url", "", "Registry API URL (e.g., https://registry.example.com)")
	rootCmd.PersistentFlags().StringVar(&authToken, "token", "", "Authentication token")
	rootCmd.PersistentFlags().StringVar(&format, "format", "", "Artifact format (npm, pypi, helm, go, apt, composer, cocoapods, brew)")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "default", "Artifact namespace")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	viper.BindPFlag("url", rootCmd.PersistentFlags().Lookup("url"))
	viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token"))
	viper.BindPFlag("format", rootCmd.PersistentFlags().Lookup("format"))
	viper.BindPFlag("namespace", rootCmd.PersistentFlags().Lookup("namespace"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".artifactadm")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

func getRegistryURL() string {
	url := viper.GetString("url")
	if url == "" {
		fmt.Fprintln(os.Stderr, "Error: Registry URL not set. Use --url flag or set in config file.")
		os.Exit(1)
	}
	return url
}

func getAuthToken() string {
	token := viper.GetString("token")
	if token == "" {
		fmt.Fprintln(os.Stderr, "Error: Auth token not set. Use --token flag or set in config file.")
		os.Exit(1)
	}
	return token
}

func getFormat() string {
	formatVal := viper.GetString("format")
	if formatVal == "" {
		formatVal = format
	}
	if formatVal == "" {
		fmt.Fprintln(os.Stderr, "Error: Format not specified. Use --format flag.")
		os.Exit(1)
	}
	return formatVal
}

func getNamespace() string {
	ns := viper.GetString("namespace")
	if ns == "" {
		ns = namespace
	}
	if ns == "" {
		ns = "default"
	}
	return ns
}

// InitConfigFile creates a default config file
func initConfigFile() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(home, ".artifactadm.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists at %s", configPath)
	}

	defaultConfig := `# ADS Registry Artifact Management Configuration

# Registry API URL
url: https://registry.example.com

# Authentication token (get from registry admin)
token: ""

# Default format (npm, pypi, helm, go, apt, composer, cocoapods, brew)
format: npm

# Default namespace
namespace: default

# Verbose output
verbose: false

# Format-specific configurations
npm:
  scope: ""
  registry: ""

pypi:
  index_url: ""

helm:
  repo_name: ""

go:
  proxy: ""

apt:
  codename: stable
  architecture: amd64

composer:
  vendor: ""

cocoapods:
  spec_repo: ""

brew:
  tap: ""
`

	return os.WriteFile(configPath, []byte(defaultConfig), 0600)
}
