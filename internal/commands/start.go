package commands

import (
	"fmt"
	"omnillm/internal/server"

	"github.com/spf13/cobra"
)

var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the LLM proxy server",
	Long:  "Start the LLM proxy server",
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

		var rateLimit *int
		if cmd.Flags().Changed("rate-limit") {
			rateLimit = &rateLimitSeconds
		}

		options := server.StartOptions{
			Port:                port,
			Host:                host,
			Verbose:             verbose,
			AccountType:         accountType,
			Manual:              manual,
			RateLimit:           rateLimit,
			RateLimitWait:       wait,
			GithubToken:         githubToken,
			ClaudeCode:          claudeCode,
			Console:             console,
			ShowToken:           showToken,
			ProxyEnv:            proxyEnv,
			Provider:            provider,
			APIKey:              apiKey,
			AllowLocalEndpoints: allowLocalEndpoints,
			EnableConfigEdit:    enableConfigEdit,
		}

		return server.RunServer(options)
	},
}

func init() {
	StartCmd.Flags().IntP("port", "p", 5005, "Port to listen on")
	StartCmd.Flags().String("host", "127.0.0.1", "IP or hostname to bind the server to")
	StartCmd.Flags().BoolP("verbose", "v", false, "Enable verbose logging")
	StartCmd.Flags().StringP("account-type", "a", "individual", "Account type to use (individual, business, enterprise)")
	StartCmd.Flags().Bool("manual", false, "Enable manual request approval")
	StartCmd.Flags().IntP("rate-limit", "r", 0, "Rate limit in seconds between requests")
	StartCmd.Flags().BoolP("wait", "w", false, "Wait instead of error when rate limit is hit")
	StartCmd.Flags().StringP("github-token", "g", "", "Provide GitHub token directly")
	StartCmd.Flags().BoolP("claude-code", "c", false, "Generate a command to launch Claude Code with proxy config")
	StartCmd.Flags().Bool("console", false, "Automatically open the admin console in your default browser")
	StartCmd.Flags().Bool("show-token", false, "Show tokens on fetch and refresh")
	StartCmd.Flags().Bool("proxy-env", false, "Initialize proxy from environment variables")
	StartCmd.Flags().String("provider", "github-copilot", "Provider to use (github-copilot, antigravity, alibaba, etc.)")
	StartCmd.Flags().String("api-key", "", "Inbound API key for protecting server routes")
	StartCmd.Flags().Bool("allow-local-endpoints", false, "Allow localhost/private OpenAI-compatible endpoints")
	StartCmd.Flags().Bool("enable-config-edit", false, "Allow editing external config files via admin API")
}
