package commands

import (
	"fmt"
	"io"
	"omnillm/internal/server"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the LLM proxy server",
	Args:  cobra.NoArgs,
	Long: `Start the OmniLLM proxy server.

The server listens on --host:--port and exposes:
  - OpenAI-compatible API at /v1/chat/completions, /v1/models, /v1/embeddings, /v1/responses
  - Anthropic-compatible API at /v1/messages
  - Admin API and UI at /admin/

Configuration precedence (highest to lowest):
  1. CLI flags
  2. Environment variables (OMNILLM_API_KEY, OMNILLM_RESPONSE_CACHE_REDIS_URL, etc.)
  3. Files in ~/.config/omnillm/

The exact-response cache stores payloads in optional Redis/Valkey. Redis failures
are fail-open: model serving continues without a SQLite cache fallback.

The inbound --api-key defaults to a generated key stored in ~/.config/omnillm/api-key.

By default, the command waits for readiness and leaves the server running in the
background. Use --foreground when running under a service manager or container.`,
	Example: `  # Start with defaults (port 5000, bound to 127.0.0.1)
  omnillm start

  # Start on a different port and allow local OpenAI-compatible endpoints
  omnillm start --port 8080 --allow-local-endpoints

  # Start with an explicit API key and verbose logging
  omnillm start --api-key my-secret --verbose

  # Start with rate limiting (1 request per 3 seconds, wait instead of error)
  omnillm start --rate-limit 3 --wait

  # Print the Claude Code client configuration after starting
  omnillm start --claude-code

  # Stay attached for Docker, systemd, or debugging
  omnillm start --foreground`,
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
		showToken, err := cmd.Flags().GetBool("show-token")
		if err != nil {
			return fmt.Errorf("get show-token flag: %w", err)
		}
		apiKey, err := cmd.Flags().GetString("api-key")
		if err != nil {
			return fmt.Errorf("get api-key flag: %w", err)
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
			Manual:                    manual,
			RateLimit:                 rateLimit,
			RateLimitWait:             wait,
			MaxConcurrentRequests:     maxConcurrent,
			GithubToken:               githubToken,
			ShowToken:                 showToken,
			APIKey:                    apiKey,
			AllowLocalEndpoints:       allowLocalEndpoints,
			EnableConfigEdit:          enableConfigEdit,
			AllowedChromeExtensionIDs: allowedChromeExtensions,
			ResponseCacheRedisURL:     redisURL,
			ResponseCacheRedisPrefix:  redisPrefix,
		}

		foreground, err := cmd.Flags().GetBool("foreground")
		if err != nil {
			return fmt.Errorf("get foreground flag: %w", err)
		}
		if foreground {
			if claudeCode {
				writeClaudeCodeConfig(cmd.OutOrStdout(), host, port)
			}
			return server.RunServer(options)
		}
		if err := startBackground(cmd, options); err != nil {
			return err
		}
		if claudeCode {
			writeClaudeCodeConfig(cmd.OutOrStdout(), host, port)
		}
		return nil
	},
}

func writeClaudeCodeConfig(out io.Writer, host string, port int) {
	clientHost := strings.TrimSpace(host)
	switch clientHost {
	case "", "0.0.0.0", "::", "[::]":
		clientHost = "127.0.0.1"
	default:
		if strings.Contains(clientHost, ":") && !strings.HasPrefix(clientHost, "[") {
			clientHost = "[" + clientHost + "]"
		}
	}
	if port <= 0 {
		port = 5000
	}
	serverURL := fmt.Sprintf("http://%s:%d", clientHost, port)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "# Claude Code Configuration")
	fmt.Fprintln(out, "# Set your OmniLLM API key (from --api-key, OMNILLM_API_KEY, or ~/.config/omnillm/api-key):")
	fmt.Fprintln(out, `export ANTHROPIC_AUTH_TOKEN="${OMNILLM_API_KEY:-$(cat ~/.config/omnillm/api-key)}"`)
	fmt.Fprintf(out, "export ANTHROPIC_BASE_URL=%s/v1\n", serverURL)
	fmt.Fprintln(out, `export OPENAI_API_KEY="${OMNILLM_API_KEY:-$(cat ~/.config/omnillm/api-key)}"`)
	fmt.Fprintf(out, "export OPENAI_BASE_URL=%s/v1\n", serverURL)
}

func environmentOverride(fallback, name string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func init() {
	addStartFlags(StartCmd)
}

func addStartFlags(cmd *cobra.Command) {
	cmd.Flags().IntP("port", "p", 5000, "Port to listen on")
	cmd.Flags().String("host", "127.0.0.1", "IP or hostname to bind the server to")
	cmd.Flags().BoolP("verbose", "v", false, "Enable verbose logging")
	cmd.Flags().Bool("manual", false, "Enable manual request approval")
	cmd.Flags().IntP("rate-limit", "r", 0, "Rate limit in seconds between requests")
	cmd.Flags().BoolP("wait", "w", false, "Wait instead of error when rate limit is hit")
	cmd.Flags().Int("max-concurrent", 0, "Max concurrent in-flight proxy requests (0 = unlimited); excess requests get HTTP 503")
	cmd.Flags().StringP("github-token", "g", "", "Provide GitHub token directly for GitHub Copilot")
	cmd.Flags().BoolP("claude-code", "c", false, "Print Claude Code client configuration after starting")
	cmd.Flags().Bool("show-token", false, "Show tokens on /token endpoint")
	cmd.Flags().String("api-key", "", "Inbound API key for protecting server routes (or set OMNILLM_API_KEY)")
	cmd.Flags().Bool("allow-local-endpoints", false, "Allow localhost/private OpenAI-compatible endpoints during onboarding")
	cmd.Flags().Bool("enable-config-edit", false, "Allow editing external config files via admin API")
	cmd.Flags().StringSlice("allow-chrome-extension", nil, "Allow specific Chrome extension IDs for CORS (repeat flag or pass comma-separated IDs)")
	cmd.Flags().String("response-cache-redis-url", "redis://127.0.0.1:6379/0", "Redis URL for exact-response cache storage")
	cmd.Flags().String("response-cache-redis-prefix", "omnillm", "Redis key prefix for exact-response cache isolation")
	cmd.Flags().Bool("foreground", false, "Run attached to the current terminal instead of in the background")
}
