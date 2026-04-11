package server

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"omnimodel/internal/database"
	"omnimodel/internal/lib/ratelimit"
	"omnimodel/internal/providers/copilot"
	"omnimodel/internal/providers/generic"
	"omnimodel/internal/providers/types"
	"omnimodel/internal/registry"
	"omnimodel/internal/routes"
)

type StartOptions struct {
	Port          int
	Verbose       bool
	AccountType   string
	Manual        bool
	RateLimit     *int
	RateLimitWait bool
	GithubToken   string
	ClaudeCode    bool
	Console       bool
	ShowToken     bool
	ProxyEnv      bool
	Provider      string
}

func RunServer(options StartOptions) error {
	// Setup logging
	setupLogging(options.Verbose)

	// Initialize database
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	configDir := filepath.Join(homeDir, ".config", "omnimodel")

	if err := database.InitializeDatabase(configDir); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize provider registry
	providerRegistry := registry.GetProviderRegistry()

	// Register default providers
	if err := registerDefaultProviders(providerRegistry, options); err != nil {
		log.Warn().Err(err).Msg("Failed to register some providers")
	}

	// Configure rate limiter
	rateLimitInterval := 0
	if options.RateLimit != nil {
		rateLimitInterval = *options.RateLimit
	}
	rl := ratelimit.NewRateLimiter(rateLimitInterval, options.RateLimitWait)
	routes.ConfigureChatCompletionOptions(rl, options.Manual)

	// Set Gin mode
	if !options.Verbose {
		gin.SetMode(gin.ReleaseMode)
	}

	r := buildRouter()

	// Claude Code mode output
	if options.ClaudeCode {
		serverURL := fmt.Sprintf("http://localhost:%d", options.Port)
		printClaudeCodeConfig(serverURL)
	}

	serverURL := fmt.Sprintf("http://localhost:%d", options.Port)
	adminURL := fmt.Sprintf("%s/admin", serverURL)

	log.Info().
		Str("url", serverURL).
		Str("admin", adminURL).
		Msg("OmniModel server starting")

	return r.Run(fmt.Sprintf(":%d", options.Port))
}

func buildRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// Structured logging middleware with request ID
	r.Use(func(c *gin.Context) {
		requestID := generateRequestID()
		c.Set("request_id", requestID)
		c.Header("X-Request-Id", requestID)

		start := time.Now()
		c.Next()

		duration := time.Since(start)
		latencyMs := duration.Milliseconds()
		status := c.Writer.Status()

		log.Info().
			Str("request_id", requestID).
			Str("method", c.Request.Method).
			Str("path", c.Request.RequestURI).
			Int("status", status).
			Int64("latency_ms", latencyMs).
			Msg("HTTP request")
	})

	// Configure CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"*"}
	r.Use(cors.New(corsConfig))

	r.SetTrustedProxies([]string{"127.0.0.1", "::1", "localhost"})

	// EventSource middleware
	r.Use(func(c *gin.Context) {
		if c.GetHeader("Accept") == "text/event-stream" {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Headers", "Cache-Control")
		}
		c.Next()
	})

	// Health check endpoints
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"message": "OmniModel server is running",
			"version": "2.0.0-go",
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.String(200, "OK")
	})

	// API routes
	api := r.Group("/")
	routes.SetupChatCompletionRoutes(api)
	routes.SetupModelRoutes(api)
	routes.SetupEmbeddingRoutes(api)
	routes.SetupUsageRoutes(api)
	routes.SetupTokenRoutes(api)

	// v1 compatibility routes
	v1 := r.Group("/v1")
	routes.SetupChatCompletionRoutes(v1)
	routes.SetupModelRoutes(v1)
	routes.SetupEmbeddingRoutes(v1)
	routes.SetupMessageRoutes(v1)
	routes.SetupResponseRoutes(v1)

	// Admin routes
	adminAPI := r.Group("/api/admin")
	routes.SetupAdminRoutes(adminAPI)
	routes.SetupVirtualModelRoutes(adminAPI)

	// Admin static files redirect
	r.GET("/admin", func(c *gin.Context) {
		c.Redirect(301, "/admin/")
	})

	return r
}

// generateRequestID creates a random request ID for correlation
func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func setupLogging(verbose bool) {
	var consoleWriter io.Writer = os.Stderr
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	if verbose {
		consoleWriter = zerolog.ConsoleWriter{Out: os.Stderr}
	}

	log.Logger = log.Output(zerolog.MultiLevelWriter(
		consoleWriter,
		sseLogWriter{source: "backend"},
	))

	log.Info().Bool("verbose", verbose).Msg("Logging configured")
}

func registerDefaultProviders(reg *registry.ProviderRegistry, options StartOptions) error {
	// Try to load saved provider instances from the database
	instanceStore := database.NewProviderInstanceStore()
	instances, err := instanceStore.GetAll()
	if err == nil && len(instances) > 0 {
		// Load providers from database
		for _, inst := range instances {
			var provider types.Provider
			switch inst.ProviderID {
			case "github-copilot":
				p := copilot.NewGitHubCopilotProvider(inst.InstanceID)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				if options.GithubToken != "" {
					p.SetupAuth(&types.AuthOptions{GithubToken: options.GithubToken})
				}
				provider = p
			default:
				p := generic.NewGenericProvider(inst.ProviderID, inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			}

			if err := reg.Register(provider, false); err != nil {
				log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to register provider")
				continue
			}
			if inst.Activated {
				reg.AddActive(inst.InstanceID)
			}
		}

		log.Info().Int("count", len(instances)).Msg("Loaded providers from database")
		return nil
	}

	// No saved providers - register default
	copilotProvider := copilot.NewGitHubCopilotProvider("github-copilot-1")

	// Try loading token from DB
	copilotProvider.LoadFromDB()

	// Override with CLI-provided token if given
	if options.GithubToken != "" {
		if err := copilotProvider.SetupAuth(&types.AuthOptions{GithubToken: options.GithubToken}); err != nil {
			log.Warn().Err(err).Msg("Failed to authenticate GitHub Copilot provider")
		}
	}

	if err := reg.Register(copilotProvider, false); err != nil {
		return fmt.Errorf("failed to register GitHub Copilot provider: %w", err)
	}

	// Always set copilot as active
	if _, err := reg.SetActive("github-copilot-1"); err != nil {
		log.Warn().Err(err).Msg("Failed to set GitHub Copilot as active provider")
	}

	log.Info().Msg("Default providers registered")
	return nil
}

func printClaudeCodeConfig(serverURL string) {
	fmt.Println("\n# Claude Code Configuration")
	fmt.Println("# Add these environment variables:")
	fmt.Printf("export OPENAI_API_KEY=dummy\n")
	fmt.Printf("export OPENAI_BASE_URL=%s/v1\n", serverURL)
	fmt.Printf("export ANTHROPIC_BASE_URL=%s/v1\n", serverURL)
	fmt.Println()
}
