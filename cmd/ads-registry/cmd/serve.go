package cmd

import (
	"context"
	"crypto/tls"
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
	"github.com/ryan/ads-registry/internal/api/auth"
	"github.com/ryan/ads-registry/internal/api/management"
	tenancyAPI "github.com/ryan/ads-registry/internal/api/tenancy"
	v2 "github.com/ryan/ads-registry/internal/api/v2"
	registryAuth "github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/automation"
	"github.com/ryan/ads-registry/internal/cache"
	"github.com/ryan/ads-registry/internal/compat"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/db/postgres"
	"github.com/ryan/ads-registry/internal/db/sqlite"
	"github.com/ryan/ads-registry/internal/health"
	"github.com/ryan/ads-registry/internal/logger"
	"github.com/ryan/ads-registry/internal/logging"
	"github.com/ryan/ads-registry/internal/policy"
	"github.com/ryan/ads-registry/internal/queue"
	"github.com/ryan/ads-registry/internal/scanner"
	"github.com/ryan/ads-registry/internal/scanner/trivy"
	"github.com/ryan/ads-registry/internal/storage"
	"github.com/ryan/ads-registry/internal/storage/local"
	"github.com/ryan/ads-registry/internal/storage/oci"
	"github.com/ryan/ads-registry/internal/storage/s3"
	"github.com/ryan/ads-registry/internal/sync"
	"github.com/ryan/ads-registry/internal/tenancy"
	"github.com/ryan/ads-registry/internal/upstreams"
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

// scannerEngineAdapter adapts scanner.Engine to queue.ScanEngine
type scannerEngineAdapter struct {
	engine scanner.Engine
}

func (a *scannerEngineAdapter) Name() string {
	return a.engine.Name()
}

func (a *scannerEngineAdapter) Scan(ctx context.Context, namespace, repo, digest string) (*queue.ScanReport, error) {
	report, err := a.engine.Scan(ctx, namespace, repo, digest)
	if err != nil {
		return nil, err
	}

	// Convert scanner.Report to queue.ScanReport
	vulns := make([]queue.Vuln, len(report.Vulnerabilities))
	for i, v := range report.Vulnerabilities {
		vulns[i] = queue.Vuln{
			ID:          v.ID,
			Package:     v.Package,
			Version:     v.Version,
			FixVersion:  v.FixVersion,
			Severity:    v.Severity,
			Description: v.Description,
		}
	}

	return &queue.ScanReport{
		Digest:          report.Digest,
		ScannerName:     report.ScannerName,
		ScannerVersion:  report.ScannerVersion,
		CreatedAt:       report.CreatedAt,
		Vulnerabilities: vulns,
	}, nil
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

	// Initialize log level from config
	logLevel := cfg.Logging.Level
	if logLevel == "" {
		logLevel = "info" // default to info
	}
	logger.SetLevel(logLevel)
	logger.Info("Log level set to: %s", logLevel)

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

	// Init Redis Cache (optional)
	var redisCache *cache.RedisCache
	if cfg.Redis.Enabled {
		logger.Info("Initializing Redis cache...")
		var err error
		redisCache, err = cache.NewRedis(cache.Config{
			Address:  cfg.Redis.Address,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
			Enabled:  cfg.Redis.Enabled,
		})
		if err != nil {
			logger.Error("Failed to initialize Redis cache", err)
			log.Fatalf("failed to init Redis: %v", err)
		}
		logger.Info(fmt.Sprintf("Redis cache initialized: %s", cfg.Redis.Address))

		// Wrap database store with caching layer
		store = cache.NewCachedStore(store, redisCache, cache.TTLConfig{
			Manifest:   time.Duration(cfg.Redis.TTL.Manifest) * time.Second,
			Signature:  time.Duration(cfg.Redis.TTL.Signature) * time.Second,
			ScanReport: time.Duration(cfg.Redis.TTL.ScanReport) * time.Second,
			Policy:     time.Duration(cfg.Redis.TTL.Policy) * time.Second,
		})
	} else {
		logger.Info("Redis caching disabled")
	}

	// 2. Init Storage
	var storageProvider storage.Provider
	switch cfg.Storage.Driver {
	case "local":
		storageProvider, err = local.New(cfg.Storage.Local.RootDirectory)
	case "s3":
		if cfg.Storage.S3 == nil {
			log.Fatal("S3 storage configuration is required when driver is 's3'")
		}
		storageProvider, err = s3.New(
			cfg.Storage.S3.Endpoint,
			cfg.Storage.S3.Region,
			cfg.Storage.S3.Bucket,
			cfg.Storage.S3.AccessKeyID,
			cfg.Storage.S3.SecretAccessKey,
			false, // usePathStyle
		)
	case "minio":
		if cfg.Storage.MinIO == nil {
			log.Fatal("MinIO storage configuration is required when driver is 'minio'")
		}
		// MinIO uses path-style addressing
		endpoint := cfg.Storage.MinIO.Endpoint
		if cfg.Storage.MinIO.UseSSL {
			endpoint = "https://" + endpoint
		} else {
			endpoint = "http://" + endpoint
		}
		storageProvider, err = s3.New(
			endpoint,
			cfg.Storage.MinIO.Region,
			cfg.Storage.MinIO.Bucket,
			cfg.Storage.MinIO.AccessKeyID,
			cfg.Storage.MinIO.SecretAccessKey,
			true, // usePathStyle for MinIO
		)
	case "oci":
		if cfg.Storage.OCI == nil {
			log.Fatal("OCI storage configuration is required when driver is 'oci'")
		}
		storageProvider, err = oci.New(oci.Config{
			Namespace:      cfg.Storage.OCI.Namespace,
			Bucket:         cfg.Storage.OCI.Bucket,
			Region:         cfg.Storage.OCI.Region,
			TenancyID:      cfg.Storage.OCI.TenancyID,
			UserID:         cfg.Storage.OCI.UserID,
			Fingerprint:    cfg.Storage.OCI.Fingerprint,
			PrivateKeyPath: cfg.Storage.OCI.PrivateKeyPath,
			PrivateKey:     cfg.Storage.OCI.PrivateKey,
		})
	default:
		log.Fatalf("unsupported storage driver: %s (supported: local, s3, minio, oci)", cfg.Storage.Driver)
	}
	if err != nil {
		logger.Error("Failed to initialize storage", err)
		log.Fatalf("failed to init storage: %v", err)
	}
	logger.Info(fmt.Sprintf("Initialized Storage: %s", cfg.Storage.Driver))

	// 3. Initialize Upstream Registry Manager
	// This manages cloud provider credentials and token refresh for AWS ECR, Oracle OCI, Docker Hub, GCP
	var upstreamManager *upstreams.Manager
	switch cfg.Database.Driver {
	case "postgres", "pgsqllite":
		// Cast to concrete postgres.PostgresStore type which implements DBStore
		if pgStore, ok := store.(*postgres.PostgresStore); ok {
			upstreamStore := upstreams.NewStoreAdapter(pgStore)
			upstreamManager = upstreams.NewManager(upstreamStore)
			logger.Info("Upstream registry manager initialized (AWS ECR, Oracle OCI, Docker Hub, GCP)")
		}
	case "sqlite3":
		// SQLite doesn't support upstream registries yet - they require PostgreSQL
		logger.Warning("Upstream registries require PostgreSQL - upstream features disabled with SQLite")
	}


	// 4. Router
	r := chi.NewRouter()

	// Initialize compatibility middleware
	compatConfig := convertCompatConfig(cfg.Compatibility)
	compatMiddleware, err := compat.NewMiddleware(&compatConfig)
	if err != nil {
		logger.Error("Failed to initialize compatibility middleware", err)
		log.Fatalf("failed to init compatibility middleware: %v", err)
	}
	if cfg.Compatibility.Enabled {
		logger.Info("Compatibility system enabled (Postfix-style client workarounds)")
	}

	// Initialize multi-tenancy middleware (PostgreSQL only)
	var tenantMiddleware *tenancy.TenantMiddleware
	var meteringService *tenancy.MeteringService
	if cfg.Database.Driver == "postgres" || cfg.Database.Driver == "pgsqllite" {
		// Get database connection for tenant middleware
		if pgStore, ok := store.(*postgres.PostgresStore); ok {
			// Extract base domain from config or environment
			baseDomain := os.Getenv("REGISTRY_BASE_DOMAIN")
			if baseDomain == "" {
				baseDomain = "registry.afterdarksys.com" // Default
			}

			tenantMiddleware = tenancy.NewTenantMiddleware(
				pgStore.DB(),
				baseDomain,
				false, // Don't require tenant (allow "default" tenant for backward compat)
			)

			// Initialize usage metering
			meteringService = tenancy.NewMeteringService(pgStore.DB())

			logger.Info(fmt.Sprintf("Multi-tenancy enabled with base domain: %s", baseDomain))
		}
	}

	// Middleware chain (order matters!)
	// 1. Tenant resolution (FIRST - resolves tenant from subdomain)
	if tenantMiddleware != nil {
		r.Use(tenantMiddleware.Middleware)
	}
	// 2. Client detection (early - enriches context)
	r.Use(compatMiddleware.ClientDetectionMiddleware)
	// 3. Logging (logs with client info if available)
	r.Use(logging.HTTPLoggingMiddleware(logger))
	// 4. Bandwidth metering (tracks usage for billing)
	if meteringService != nil {
		meteringMiddleware := tenancy.NewBandwidthMeteringMiddleware(meteringService)
		r.Use(meteringMiddleware.Middleware)
	}
	// 5. Compatibility workarounds (applies fixes based on detected client)
	r.Use(compatMiddleware.CompatibilityMiddleware)
	// 6. Recovery (catch panics)
	r.Use(middleware.Recoverer)
	// 7. Rate limiting (increased to 10000 for OCI migration)
	r.Use(httprate.LimitByIP(10000, 1*time.Minute))

	// Management & Observability
	healthHandler := health.NewHandler("1.0.0")
	// Register health checks
	healthHandler.RegisterLiveness("uptime", health.UptimeChecker(time.Now()))
	// TODO: Add database and storage checkers for readiness

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health/live", healthHandler.Liveness)
	r.Get("/health/ready", healthHandler.Readiness)

	// Native OCI Dist API

	// Init Security Providers
	// 1. Scanner engine mapping
	wd := webhooks.NewDispatcher(cfg.Webhooks)
	engines := []scanner.Engine{
		trivy.New("/tmp/trivy-cache"),
		scanner.NewStaticAnalyzer(storageProvider), // Static analysis with Semgrep
	}

	// Choose scanner implementation based on database driver and queue configuration
	var riverWorker *scanner.RiverWorker
	var queueClient *queue.Client

	if cfg.Queue.Enabled && (cfg.Database.Driver == "postgres" || cfg.Database.Driver == "pgsqllite") {
		// Use River for PostgreSQL
		logger.Info("Initializing River job queue for vulnerability scanning")

		// Run River migrations
		logger.Info("Running River database migrations...")
		if err := queue.RunMigrations(context.Background(), cfg.Database.DSN); err != nil {
			logger.Error("Failed to run River migrations", err)
			log.Fatalf("failed to run River migrations: %v", err)
		}
		logger.Info("River migrations completed successfully")

		// Convert engines to queue.ScanEngine
		queueEngines := make([]queue.ScanEngine, len(engines))
		for i, eng := range engines {
			queueEngines[i] = &scannerEngineAdapter{engine: eng}
		}

		var err error
		queueClient, err = queue.NewClient(
			context.Background(),
			cfg.Database.DSN,
			cfg.Queue.DefaultQueue,
			cfg.Queue.VulnerabilityQueue,
			cfg.Queue.PeriodicQueue,
			store,
			storageProvider,
			queueEngines,
			wd,
			upstreamManager,
		)
		if err != nil {
			logger.Error("Failed to initialize River queue", err)
			log.Fatalf("failed to init River queue: %v", err)
		}

		riverWorker = scanner.NewRiverWorker(queueClient)
		if err := riverWorker.Start(context.Background()); err != nil {
			logger.Error("Failed to start River worker", err)
			log.Fatalf("failed to start River worker: %v", err)
		}
		logger.Info("River job queue started successfully")
	} else {
		// Use channel-based scanner for SQLite or when queue is disabled
		logger.Info("Using channel-based scanner (SQLite mode)")
		channelWorker := scanner.NewWorker(store, storageProvider, engines, wd)
		channelWorker.Start(context.Background(), 2)
	}

	// 2. CEL Policy Admisson Rules
	enf, err := policy.NewEnforcer(store)
	if err != nil {
		log.Fatalf("failed to init CEL enforcer: %v", err)
	}
	// Add default example whitelist/blacklist rules
	enf.AddRule(`request.namespace != "blacklist"`)
	enf.AddRule(`request.method == "GET" || request.namespace == "trusted"`)

	// 3. Initialize Auth Token Service
	tokenService, err := registryAuth.NewTokenService(cfg.Auth)
	if err != nil {
		logger.Error("Failed to initialize token service", err)
		log.Fatalf("failed to init token service: %v", err)
	}
	logger.Info("Token service initialized successfully")

	// 4. Starlark Embedded Automation
	starEng := automation.NewEngine()

	// 5. Starlark Cron Scheduler (for automated tasks like upstream token refresh)
	cronScheduler := automation.NewCronScheduler(starEng, "scripts")
	cronScheduler.Start()
	logger.Info("Starlark cron scheduler started")

	// 6. Master-Peer Sync Engine
	syncManager := sync.NewManager(cfg.Peers, starEng)
	syncManager.Start(2) // 2 concurrent sync workers
	defer syncManager.Stop()

	// 7. Vulnerability Scanner (DarkScan via darkapi.io)
	scannerService := scanner.NewService(store, cfg.DarkScan)
	if scannerService.IsEnabled() {
		logger.Info("DarkScan vulnerability scanner enabled")
	}

	v2api := v2.NewRouter(store, storageProvider, tokenService, enf, starEng, upstreamManager, syncManager, scannerService) // passing enf for policy control, starEng for automation, upstreamManager for proxy, syncManager for peer replication, scannerService for vulnerability scanning
	v2api.Register(r)

	// OAuth2 Authentication API for Web UI
	oauth2Router := auth.NewOAuth2Router(store, tokenService)
	oauth2Router.Register(r)

	// OIDC/SSO Authentication (Authentik)
	oidcHandler, err := auth.NewOIDCHandler(cfg.Auth.OIDC, store, tokenService)
	if err != nil {
		logger.Error("Failed to initialize OIDC handler", err)
		log.Fatalf("failed to init OIDC: %v", err)
	}
	if oidcHandler != nil {
		oidcHandler.Register(r)
		logger.Info("OIDC/SSO authentication enabled")
	}

	// Admin Dashboard Management API
	managementRouter := management.NewRouter(store, tokenService, enf, starEng)
	managementRouter.Register(r)

	// Multi-Tenancy Management API (PostgreSQL only)
	if cfg.Database.Driver == "postgres" || cfg.Database.Driver == "pgsqllite" {
		if pgStore, ok := store.(*postgres.PostgresStore); ok {
			tenancyRouter := tenancyAPI.NewRouter(pgStore.DB())
			tenancyRouter.RegisterRoutes(r)

			// OIDC Authentication endpoints
			baseURL := os.Getenv("REGISTRY_BASE_URL")
			if baseURL == "" {
				baseURL = "https://registry.afterdarksys.com"
			}
			oidcAuthHandler := tenancyAPI.NewOIDCAuthHandler(pgStore.DB(), store, tokenService, baseURL)
			oidcAuthHandler.RegisterRoutes(r)

			logger.Info("Multi-tenancy management API enabled")
		}
	}

	// React SPA Server
	distDir := "web/dist"
	fs := http.FileServer(http.Dir(distDir))
	r.HandleFunc("/*", func(w http.ResponseWriter, req *http.Request) {
		if _, err := os.Stat(filepath.Join(distDir, req.URL.Path)); os.IsNotExist(err) {
			http.ServeFile(w, req, filepath.Join(distDir, "index.html"))
			return
		}
		fs.ServeHTTP(w, req)
	})

	// 4. Server
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port),
		Handler:           r,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
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
			MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
			TLSNextProto:      make(map[string]func(*http.Server, *tls.Conn, http.Handler)), // Disable HTTP/2
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

	// Stop cron scheduler
	cronScheduler.Stop()
	logger.Info("Cron scheduler stopped")

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

	// Stop River worker if running
	if riverWorker != nil {
		logger.Info("Stopping River job queue...")
		if err := riverWorker.Stop(ctx); err != nil {
			logger.Error("Error stopping River worker", err)
		}
		if err := queueClient.Close(); err != nil {
			logger.Error("Error closing River queue client", err)
		}
	}

	// Close Redis connection
	if redisCache != nil {
		logger.Info("Closing Redis connection...")
		if err := redisCache.Close(); err != nil {
			logger.Error("Error closing Redis", err)
		}
	}

	// Close database connection
	logger.Info("Closing database connection...")
	if err := store.Close(); err != nil {
		logger.Error("Error closing database", err)
	}

	logger.Info("Server exited cleanly")
}

// convertCompatConfig converts config.CompatibilityConfig to compat.Config
func convertCompatConfig(cfg config.CompatibilityConfig) compat.Config {
	return compat.Config{
		Enabled: cfg.Enabled,
		DockerClientWorkarounds: compat.DockerWorkarounds{
			EnableDocker29ManifestFix: cfg.DockerClientWorkarounds.EnableDocker29ManifestFix,
			ForceHTTP1ForManifests:    cfg.DockerClientWorkarounds.ForceHTTP1ForManifests,
			DisableChunkedEncoding:    cfg.DockerClientWorkarounds.DisableChunkedEncoding,
			MaxManifestSize:           cfg.DockerClientWorkarounds.MaxManifestSize,
			ExtraFlushes:              cfg.DockerClientWorkarounds.ExtraFlushes,
			HeaderWriteDelay:          cfg.DockerClientWorkarounds.HeaderWriteDelay,
		},
		ProtocolEmulation: compat.ProtocolEmulation{
			EmulateDockerRegistryV2: cfg.ProtocolEmulation.EmulateDockerRegistryV2,
			EmulateDistributionV3:   cfg.ProtocolEmulation.EmulateDistributionV3,
			ExposeOCIFeatures:       cfg.ProtocolEmulation.ExposeOCIFeatures,
			StrictMode:              cfg.ProtocolEmulation.StrictMode,
		},
		BrokenClientHacks: compat.BrokenClientHacks{
			PodmanDigestWorkaround:  cfg.BrokenClientHacks.PodmanDigestWorkaround,
			SkopeoLayerReuse:        cfg.BrokenClientHacks.SkopeoLayerReuse,
			CraneManifestFormat:     cfg.BrokenClientHacks.CraneManifestFormat,
			ContainerdContentLength: cfg.BrokenClientHacks.ContainerdContentLength,
			BuildkitParallelUpload:  cfg.BrokenClientHacks.BuildkitParallelUpload,
			NerdctlMissingHeaders:   cfg.BrokenClientHacks.NerdctlMissingHeaders,
		},
		TLSCompatibility: compat.TLSCompatibility{
			MinTLSVersion:        cfg.TLSCompatibility.MinTLSVersion,
			EnableLegacyCiphers:  cfg.TLSCompatibility.EnableLegacyCiphers,
			HTTP2Enabled:         cfg.TLSCompatibility.HTTP2Enabled,
			ForceHTTP1ForClients: cfg.TLSCompatibility.ForceHTTP1ForClients,
			ALPNProtocols:        cfg.TLSCompatibility.ALPNProtocols,
		},
		HeaderWorkarounds: compat.HeaderWorkarounds{
			AlwaysSendDistributionAPIVersion: cfg.HeaderWorkarounds.AlwaysSendDistributionAPIVersion,
			ContentTypeFixups:                cfg.HeaderWorkarounds.ContentTypeFixups,
			LocationHeaderFormat:             cfg.HeaderWorkarounds.LocationHeaderFormat,
			EnableCORS:                       cfg.HeaderWorkarounds.EnableCORS,
			AcceptMalformedAccept:            cfg.HeaderWorkarounds.AcceptMalformedAccept,
			NormalizeDigestHeader:            cfg.HeaderWorkarounds.NormalizeDigestHeader,
		},
		RateLimitExceptions: compat.RateLimitExceptions{
			TrustedRegistries:      cfg.RateLimitExceptions.TrustedRegistries,
			CICDUserAgents:         cfg.RateLimitExceptions.CICDUserAgents,
			TrustedIPRanges:        cfg.RateLimitExceptions.TrustedIPRanges,
			BypassForAuthenticated: cfg.RateLimitExceptions.BypassForAuthenticated,
		},
		Observability: compat.ObservabilityConfig{
			LogWorkarounds:     cfg.Observability.LogWorkarounds,
			LogClientDetection: cfg.Observability.LogClientDetection,
			EnableMetrics:      cfg.Observability.EnableMetrics,
			MetricsPrefix:      cfg.Observability.MetricsPrefix,
			LogSampleRate:      cfg.Observability.LogSampleRate,
			LogSuccessOnly:     cfg.Observability.LogSuccessOnly,
		},
	}
}
