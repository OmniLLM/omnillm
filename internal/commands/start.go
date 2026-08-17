package commands

import (
	"fmt"
	"omnillm/internal/server"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the LLM proxy server",
	Long: `Start the OmniLLM proxy server.

The server listens on --host:--port and exposes:
  - OpenAI-compatible API at /v1/chat/completions, /v1/models, /v1/embeddings
  - Anthropic-compatible API at /v1/messages
  - Admin API and UI at /admin/

Configuration precedence (highest to lowest):
  1. CLI flags
  2. Environment variables (OMNILLM_SERVER, OMNILLM_API_KEY, OMNILLM_RESPONSE_CACHE_REDIS_URL, etc.)
  3. Files in ~/.config/omnillm/

The exact-response cache stores payloads in optional Redis/Valkey. Redis failures
are fail-open: model serving continues without a SQLite cache fallback.

The inbound --api-key defaults to a generated key stored in ~/.config/omnillm/api-key.`,
	Example: `  # Start with defaults (port 5000, github-copilot provider)
  omnillm start

  # Start on a different port with a specific provider
  omnillm start --port 8080 --provider openai-compatible

  # Start with an explicit API key and verbose logging
  omnillm start --api-key my-secret --verbose

  # Start with rate limiting (1 request per 3 seconds, wait instead of error)
  omnillm start --rate-limit 3 --wait

  # Print the Claude Code launch command after starting
  omnillm start --claude-code`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, err := cmd.Flags().GetInt("port")
		if err != nil {
			return fmt.Errorf("get port flag: %w", err)
		}
		host, err := cmd.Flags().GetString("host")
		if err != nil {
			return fmt.Errorf("get host flag: %w", err)
		}

		verbose, err := cmd.Flags().GetBool("verbose")
		if err != nil {
			return fmt.Errorf("get verbose flag: %w", err)
		}
		accountType, err := cmd.Flags().GetString("account-type")
		if err != nil {
			return fmt.Errorf("get account-type flag: %w", err)
		}
		manual, err := cmd.Flags().GetBool("manual")
		if err != nil {
			return fmt.Errorf("get manual flag: %w", err)
		}
		rateLimitSeconds, err := cmd.Flags().GetInt("rate-limit")
		if err != nil {
			return fmt.Errorf("get rate-limit flag: %w", err)
		}
		wait, err := cmd.Flags().GetBool("wait")
		if err != nil {
			return fmt.Errorf("get wait flag: %w", err)
		}
		maxConcurrent, err := cmd.Flags().GetInt("max-concurrent")
		if err != nil {
			return fmt.Errorf("get max-concurrent flag: %w", err)
		}
		githubToken, err := cmd.Flags().GetString("github-token")
		if err != nil {
			return fmt.Errorf("get github-token flag: %w", err)
		}
		claudeCode, err := cmd.Flags().GetBool("claude-code")
		if err != nil {
			return fmt.Errorf("get claude-code flag: %w", err)
		}
		console, err := cmd.Flags().GetBool("console")
		if err != nil {
			return fmt.Errorf("get console flag: %w", err)
		}
		showToken, err := cmd.Flags().GetBool("show-token")
		if err != nil {
			return fmt.Errorf("get show-token flag: %w", err)
		}
		proxyEnv, err := cmd.Flags().GetBool("proxy-env")
		if err != nil {
			return fmt.Errorf("get proxy-env flag: %w", err)
		}
		apiKey, err := cmd.Flags().GetString("api-key")
		if err != nil {
			return fmt.Errorf("get api-key flag: %w", err)
		}
		provider, err := cmd.Flags().GetString("provider")
		if err != nil {
			return fmt.Errorf("get provider flag: %w", err)
		}
		allowLocalEndpoints, err := cmd.Flags().GetBool("allow-local-endpoints")
		if err != nil {
			return fmt.Errorf("get allow-local-endpoints flag: %w", err)
		}
		enableConfigEdit, err := cmd.Flags().GetBool("enable-config-edit")
		if err != nil {
			return fmt.Errorf("get enable-config-edit flag: %w", err)
		}
		allowedChromeExtensions, err := cmd.Flags().GetStringSlice("allow-chrome-extension")
		if err != nil {
			return fmt.Errorf("get allow-chrome-extension flag: %w", err)
		}
		redisURL, err := cmd.Flags().GetString("response-cache-redis-url")
		if err != nil {
			return fmt.Errorf("get response-cache-redis-url flag: %w", err)
		}
		if !cmd.Flags().Changed("response-cache-redis-url") {
			redisURL = environmentOverride(redisURL, "OMNILLM_RESPONSE_CACHE_REDIS_URL")
		}
		redisPrefix, err := cmd.Flags().GetString("response-cache-redis-prefix")
		if err != nil {
			return fmt.Errorf("get response-cache-redis-prefix flag: %w", err)
		}
		if !cmd.Flags().Changed("response-cache-redis-prefix") {
			redisPrefix = environmentOverride(redisPrefix, "OMNILLM_RESPONSE_CACHE_REDIS_PREFIX")
		}

		var rateLimit *int
		if cmd.Flags().Changed("rate-limit") {
			rateLimit = &rateLimitSeconds
		}

		options := server.StartOptions{
			Port:                      port,
			Host:                      host,
			Verbose:                   verbose,
			AccountType:               accountType,
			Manual:                    manual,
			RateLimit:                 rateLimit,
			RateLimitWait:             wait,
			MaxConcurrentRequests:     maxConcurrent,
			GithubToken:               githubToken,
			ClaudeCode:                claudeCode,
			Console:                   console,
			ShowToken:                 showToken,
			ProxyEnv:                  proxyEnv,
			Provider:                  provider,
			APIKey:                    apiKey,
			AllowLocalEndpoints:       allowLocalEndpoints,
			EnableConfigEdit:          enableConfigEdit,
			AllowedChromeExtensionIDs: allowedChromeExtensions,
			ResponseCacheRedisURL:     redisURL,
			ResponseCacheRedisPrefix:  redisPrefix,
		}

		return server.RunServer(options)
	},
}

func environmentOverride(fallback, name string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func init() {
	StartCmd.Flags().IntP("port", "p", 5000, "Port to listen on")
	StartCmd.Flags().String("host", "127.0.0.1", "IP or hostname to bind the server to")
	StartCmd.Flags().BoolP("verbose", "v", false, "Enable verbose logging")
	StartCmd.Flags().StringP("account-type", "a", "individual", "Account type to use (individual, business, enterprise)")
	StartCmd.Flags().Bool("manual", false, "Enable manual request approval")
	StartCmd.Flags().IntP("rate-limit", "r", 0, "Rate limit in seconds between requests")
	StartCmd.Flags().BoolP("wait", "w", false, "Wait instead of error when rate limit is hit")
	StartCmd.Flags().Int("max-concurrent", 0, "Max concurrent in-flight proxy requests (0 = unlimited); excess requests get HTTP 503")
	StartCmd.Flags().StringP("github-token", "g", "", "Provide GitHub token directly")
	StartCmd.Flags().BoolP("claude-code", "c", false, "Generate a command to launch Claude Code with proxy config")
	StartCmd.Flags().Bool("console", false, "Automatically open the admin console in your default browser")
	StartCmd.Flags().Bool("show-token", false, "Show tokens on fetch and refresh")
	StartCmd.Flags().Bool("proxy-env", false, "Initialize proxy from environment variables")
	StartCmd.Flags().String("provider", "github-copilot", "Provider to use (github-copilot, antigravity, alibaba, etc.)")
	StartCmd.Flags().String("api-key", "", "Inbound API key for protecting server routes")
	StartCmd.Flags().Bool("allow-local-endpoints", false, "Allow localhost/private OpenAI-compatible endpoints")
	StartCmd.Flags().Bool("enable-config-edit", false, "Allow editing external config files via admin API")
	StartCmd.Flags().StringSlice("allow-chrome-extension", nil, "Allow specific Chrome extension IDs for CORS (repeat flag or pass comma-separated IDs)")
	StartCmd.Flags().String("response-cache-redis-url", "redis://127.0.0.1:6379/0", "Redis URL for exact-response cache storage")
	StartCmd.Flags().String("response-cache-redis-prefix", "omnillm", "Redis key prefix for exact-response cache isolation")
}
