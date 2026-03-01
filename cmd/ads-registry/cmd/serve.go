package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	v2 "github.com/ryan/ads-registry/internal/api/v2"
	"github.com/ryan/ads-registry/internal/automation"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/db/postgres"
	"github.com/ryan/ads-registry/internal/db/sqlite"
	"github.com/ryan/ads-registry/internal/health"
	"github.com/ryan/ads-registry/internal/logging"
	"github.com/ryan/ads-registry/internal/policy"
	"github.com/ryan/ads-registry/internal/scanner"
	"github.com/ryan/ads-registry/internal/scanner/trivy"
	"github.com/ryan/ads-registry/internal/storage"
	"github.com/ryan/ads-registry/internal/storage/local"
	"github.com/ryan/ads-registry/internal/vault"
	"github.com/ryan/ads-registry/internal/webhooks"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ads-registry HTTP server",
	Run: func(cmd *cobra.Command, args []string) {
		runServer()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServer() {
	// Load configuration from file
	cfg, err := config.LoadFile("config.json")
	if err != nil {
		log.Printf("Warning: failed to load config.json (%v), using default scaffold config", err)
		cfg = &config.Config{
			Server: config.ServerConfig{
				Address:           "0.0.0.0",
				Port:              5005,
				ReadTimeout:       10 * time.Second,
				WriteTimeout:      10 * time.Second,
				IdleTimeout:       60 * time.Second,
				ReadHeaderTimeout: 5 * time.Second,
				TLS: config.TLSConfig{
					Enabled:  false,
					Port:     5006,
					CertFile: "certs/server.crt",
					KeyFile:  "certs/server.key",
				},
			},
			Database: config.DatabaseConfig{
				Driver:          "sqlite3",
				DSN:             "data/registry.db",
				MaxOpenConns:    100,
				MaxIdleConns:    10,
				ConnMaxLifetime: 1 * time.Hour,
			},
			Storage: config.StorageConfig{
				Driver: "local",
				Local: &config.LocalStore{
					RootDirectory: "data/blobs",
				},
			},
		}
	}

	// Initialize enterprise logging
	logCfg := logging.Config{
		SyslogEnabled:  cfg.Logging.Syslog.Enabled,
		SyslogServer:   cfg.Logging.Syslog.Server,
		SyslogTag:      cfg.Logging.Syslog.Tag,
		SyslogPriority: cfg.Logging.Syslog.Priority,
		Elasticsearch: logging.ElasticsearchConfig{
			Enabled:  cfg.Logging.Elasticsearch.Enabled,
			Endpoint: cfg.Logging.Elasticsearch.Endpoint,
			Index:    cfg.Logging.Elasticsearch.Index,
			Username: cfg.Logging.Elasticsearch.Username,
			Password: cfg.Logging.Elasticsearch.Password,
		},
	}

	logger, err := logging.NewLogger(logCfg)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	if err := logging.InitGlobalLogger(logCfg); err != nil {
		log.Fatalf("Failed to initialize global logger: %v", err)
	}

	logger.Info("ADS Container Registry starting up")
	if cfg.Logging.Syslog.Enabled {
		logger.Info(fmt.Sprintf("Syslog enabled: %s", cfg.Logging.Syslog.Server))
	}
	if cfg.Logging.Elasticsearch.Enabled {
		logger.Info(fmt.Sprintf("Elasticsearch logging enabled: %s", cfg.Logging.Elasticsearch.Endpoint))
	}

	// Initialize Vault integration if enabled
	if cfg.Vault.Enabled {
		logger.Info(fmt.Sprintf("Vault integration enabled: %s", cfg.Vault.Address))
		vaultClient := vault.NewClient(cfg.Vault.Address, cfg.Vault.Token, cfg.Vault.MountPath)

		// Health check Vault connectivity
		if err := vaultClient.HealthCheck(); err != nil {
			logger.Error("Vault health check failed", err)
			log.Fatalf("Failed to connect to Vault: %v", err)
		}

		logger.Info("Vault health check passed")

		// Note: JWT key retrieval from Vault would happen in auth token service initialization
		// For now we're just validating connectivity
	}

	// 1. Init Database
	var store db.Store
	switch cfg.Database.Driver {
	case "sqlite3":
		store, err = sqlite.New(cfg.Database)
	case "postgres", "pgsqllite":
		store, err = postgres.New(cfg.Database)
	default:
		log.Fatalf("unsupported database driver %s", cfg.Database.Driver)
	}
	if err != nil {
		logger.Error("Failed to initialize database", err)
		log.Fatalf("failed to init db: %v", err)
	}
	defer store.Close()
	logger.Info(fmt.Sprintf("Initialized Database: %s", cfg.Database.Driver))

	// 2. Init Storage
	var storageProvider storage.Provider
	if cfg.Storage.Driver == "local" {
		storageProvider, err = local.New(cfg.Storage.Local.RootDirectory)
	} else {
		log.Fatalf("unsupported storage driver %s", cfg.Storage.Driver)
	}
	if err != nil {
		logger.Error("Failed to initialize storage", err)
		log.Fatalf("failed to init storage: %v", err)
	}
	logger.Info(fmt.Sprintf("Initialized Storage: %s", cfg.Storage.Driver))

	// 3. Router
	r := chi.NewRouter()
	r.Use(logging.HTTPLoggingMiddleware(logger)) // Enterprise logging middleware
	r.Use(middleware.Recoverer)
	r.Use(httprate.LimitByIP(100, 1*time.Minute))

	// Management & Observability
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health/live", health.LivenessHandler())
	r.Get("/health/ready", health.ReadinessHandler(store, storageProvider))

	// Native OCI Dist API

	// Init Security Providers
	// 1. Scanner engine mapping
	wd := webhooks.NewDispatcher(cfg.Webhooks)
	engines := []scanner.Engine{
		trivy.New("/tmp/trivy-cache"),
	}
	scanWorker := scanner.NewWorker(store, storageProvider, engines, wd)
	scanWorker.Start(context.Background(), 2)

	// 2. CEL Policy Admisson Rules
	enf, err := policy.NewEnforcer(store)
	if err != nil {
		log.Fatalf("failed to init CEL enforcer: %v", err)
	}
	// Add default example whitelist/blacklist rules
	enf.AddRule(`request.namespace != "blacklist"`)
	enf.AddRule(`request.method == "GET" || request.namespace == "trusted"`)

	// 3. Starlark Embedded Automation
	starEng := automation.NewEngine()

	v2api := v2.NewRouter(store, storageProvider, nil, enf, starEng) // passing enf for policy control, starEng for automation
	v2api.Register(r)

	// Admin Dashboard API Stub
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"total_repos": 142, "storage_used": "42.5 GB", "critical_vulns": 12, "policy_blocks": 842}`))
		})
	})

	// React SPA Server
	distDir := "web/dist"
	fs := http.FileServer(http.Dir(distDir))
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(distDir, r.URL.Path)); os.IsNotExist(err) {
			http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})

	// 4. Server
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port),
		Handler:           r,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
	}

	go func() {
		logger.Info(fmt.Sprintf("Starting registry on %s:%d", cfg.Server.Address, cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Critical("Server error", err)
			log.Fatalf("server error: %v", err)
		}
	}()

	var srvTLS *http.Server
	if cfg.Server.TLS.Enabled {
		srvTLS = &http.Server{
			Addr:              fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.TLS.Port),
			Handler:           r,
			ReadTimeout:       cfg.Server.ReadTimeout,
			WriteTimeout:      cfg.Server.WriteTimeout,
			IdleTimeout:       cfg.Server.IdleTimeout,
			ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		}
		go func() {
			logger.Info(fmt.Sprintf("Starting secure registry on %s:%d", cfg.Server.Address, cfg.Server.TLS.Port))
			// It is critical that users provision legitimate certs to CertFile and KeyFile for this to bind successfully
			if err := srvTLS.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				logger.Critical("Secure server error", err)
				log.Fatalf("secure server error: %v", err)
			}
		}()
	}

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Warning("Received shutdown signal, shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("HTTP Server forced to shutdown", err)
	} else {
		logger.Info("HTTP Server shutdown gracefully")
	}

	if srvTLS != nil {
		if err := srvTLS.Shutdown(ctx); err != nil {
			logger.Error("HTTPS Server forced to shutdown", err)
		} else {
			logger.Info("HTTPS Server shutdown gracefully")
		}
	}

	// Close database connection
	logger.Info("Closing database connection...")
	if err := store.Close(); err != nil {
		logger.Error("Error closing database", err)
	}

	logger.Info("Server exited cleanly")
}
