package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile    string
	apiURL     string
	adminToken string
)

var rootCmd = &cobra.Command{
	Use:   "adsradm",
	Short: "Remote administration tool for ADS Registry",
	Long: `adsradm is a remote administration CLI for managing ADS Registry instances.
It communicates with the registry's management API to perform administrative tasks
such as user management, upstream configuration, policy management, and monitoring.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.adsradm.yaml)")
	rootCmd.PersistentFlags().StringVar(&apiURL, "url", "", "Registry API URL (e.g., https://registry.example.com)")
	rootCmd.PersistentFlags().StringVar(&adminToken, "token", "", "Admin authentication token")

	viper.BindPFlag("url", rootCmd.PersistentFlags().Lookup("url"))
	viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token"))
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
		viper.SetConfigName(".adsradm")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

func getAPIURL() string {
	url := viper.GetString("url")
	if url == "" {
		fmt.Fprintln(os.Stderr, "Error: Registry URL not set. Use --url flag or set in config file.")
		os.Exit(1)
	}
	return url
}

func getAdminToken() string {
	token := viper.GetString("token")
	if token == "" {
		fmt.Fprintln(os.Stderr, "Error: Admin token not set. Use --token flag or set in config file.")
		os.Exit(1)
	}
	return token
}

// InitConfigFile creates a default config file
func initConfigFile() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(home, ".adsradm.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists at %s", configPath)
	}

	defaultConfig := `# ADS Registry Remote Administration Configuration
# Registry API URL
url: https://registry.example.com

# Admin authentication token
# Get this from: ads-registry create-user admin --scopes=admin
token: ""
`

	return os.WriteFile(configPath, []byte(defaultConfig), 0600)
}
