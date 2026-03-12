package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db/postgres"
	"github.com/ryan/ads-registry/internal/upstreams"
	"github.com/spf13/cobra"
)

var addUpstreamCmd = &cobra.Command{
	Use:   "add-upstream [name]",
	Short: "Add an upstream container registry (AWS ECR, Oracle OCI, Docker Hub)",
	Long: `Add an upstream container registry to proxy and cache images from.

Supported providers:
  - aws       AWS Elastic Container Registry (ECR)
  - oracle    Oracle Cloud Infrastructure Registry (OCIR)
  - dockerhub Docker Hub

Examples:
  # Add AWS ECR
  ads-registry add-upstream my-ecr --type aws --aws-region us-west-2 --aws-account-id 123456789012 --aws-access-key AKIA... --aws-secret-key ...

  # Add Oracle OCI
  ads-registry add-upstream my-oci --type oracle --oci-region us-phoenix-1 --oci-user-id ocid1.user... --oci-private-key "$(cat ~/.oci/key.pem)"

  # Add Docker Hub
  ads-registry add-upstream dockerhub --type dockerhub --dockerhub-username myuser --dockerhub-password mytoken
`,
	Args: cobra.ExactArgs(1),
	Run:  runAddUpstream,
}

var (
	upstreamType string

	// AWS flags
	awsRegion          string
	awsAccountID       string
	awsAccessKeyID     string
	awsSecretAccessKey string

	// Oracle flags
	ociRegion     string
	ociTenancyID  string
	ociUserID     string
	ociFingerprint string
	ociPrivateKey string

	// Docker Hub flags
	dockerhubUsername string
	dockerhubPassword string
)

func init() {
	rootCmd.AddCommand(addUpstreamCmd)

	addUpstreamCmd.Flags().StringVar(&upstreamType, "type", "", "Upstream type (aws, oracle, dockerhub)")
	addUpstreamCmd.MarkFlagRequired("type")

	// AWS flags
	addUpstreamCmd.Flags().StringVar(&awsRegion, "aws-region", "", "AWS region (e.g., us-west-2)")
	addUpstreamCmd.Flags().StringVar(&awsAccountID, "aws-account-id", "", "AWS account ID")
	addUpstreamCmd.Flags().StringVar(&awsAccessKeyID, "aws-access-key", "", "AWS access key ID")
	addUpstreamCmd.Flags().StringVar(&awsSecretAccessKey, "aws-secret-key", "", "AWS secret access key")

	// Oracle flags
	addUpstreamCmd.Flags().StringVar(&ociRegion, "oci-region", "", "Oracle region (e.g., us-phoenix-1)")
	addUpstreamCmd.Flags().StringVar(&ociTenancyID, "oci-tenancy-id", "", "Oracle tenancy OCID")
	addUpstreamCmd.Flags().StringVar(&ociUserID, "oci-user-id", "", "Oracle user OCID")
	addUpstreamCmd.Flags().StringVar(&ociFingerprint, "oci-fingerprint", "", "Oracle API key fingerprint")
	addUpstreamCmd.Flags().StringVar(&ociPrivateKey, "oci-private-key", "", "Oracle private key (PEM format)")

	// Docker Hub flags
	addUpstreamCmd.Flags().StringVar(&dockerhubUsername, "dockerhub-username", "", "Docker Hub username")
	addUpstreamCmd.Flags().StringVar(&dockerhubPassword, "dockerhub-password", "", "Docker Hub password or access token")
}

func runAddUpstream(cmd *cobra.Command, args []string) {
	name := args[0]

	// Load config
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "config.json"
	}
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize database (upstream registries require PostgreSQL)
	if cfg.Database.Driver != "postgres" && cfg.Database.Driver != "pgsqllite" {
		log.Fatal("Upstream registries require PostgreSQL database - SQLite is not supported for this feature")
	}

	store, err := postgres.New(cfg.Database)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer store.Close()

	// Build credential config based on type
	credConfig := &upstreams.CredentialConfig{}

	switch upstreamType {
	case "aws":
		if awsRegion == "" || awsAccountID == "" || awsAccessKeyID == "" || awsSecretAccessKey == "" {
			log.Fatal("AWS upstream requires: --aws-region, --aws-account-id, --aws-access-key, --aws-secret-key")
		}
		credConfig.AWSRegion = awsRegion
		credConfig.AWSAccountID = awsAccountID
		credConfig.AWSAccessKeyID = awsAccessKeyID
		credConfig.AWSSecretAccessKey = awsSecretAccessKey

	case "oracle":
		if ociRegion == "" || ociUserID == "" || ociPrivateKey == "" {
			log.Fatal("Oracle upstream requires: --oci-region, --oci-user-id, --oci-private-key")
		}
		credConfig.OCIRegion = ociRegion
		credConfig.OCITenancyID = ociTenancyID
		credConfig.OCIUserID = ociUserID
		credConfig.OCIFingerprint = ociFingerprint
		credConfig.OCIPrivateKey = ociPrivateKey

	case "dockerhub":
		if dockerhubUsername == "" || dockerhubPassword == "" {
			log.Fatal("Docker Hub upstream requires: --dockerhub-username, --dockerhub-password")
		}
		credConfig.DockerHubUsername = dockerhubUsername
		credConfig.DockerHubPassword = dockerhubPassword

	default:
		log.Fatalf("unsupported upstream type: %s (must be aws, oracle, or dockerhub)", upstreamType)
	}

	// Create upstream manager with store adapter
	storeAdapter := upstreams.NewStoreAdapter(store)
	manager := upstreams.NewManager(storeAdapter)

	// Add upstream
	upstream, err := manager.AddUpstream(context.Background(), name, upstreams.UpstreamType(upstreamType), credConfig)
	if err != nil {
		log.Fatalf("failed to add upstream: %v", err)
	}

	fmt.Printf("✅ Successfully added upstream registry: %s (%s)\n", upstream.Name, upstream.Type)
	fmt.Printf("   Endpoint: %s\n", upstream.Endpoint)
	fmt.Printf("   Region: %s\n", upstream.Region)
	fmt.Printf("   Token expires: %s\n", upstream.TokenExpiry.Format("2006-01-02 15:04:05"))
	fmt.Printf("\nYou can now proxy images through this upstream:\n")
	fmt.Printf("   docker pull localhost:5005/%s/myimage:latest\n", name)
}
