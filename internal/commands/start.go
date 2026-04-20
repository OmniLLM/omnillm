package commands

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"omnillm/internal/server"
)

var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the LLM proxy server",
	Long:  "Start the LLM proxy server",
	RunE: func(cmd *cobra.Command, args []string) error {
		portStr, _ := cmd.Flags().GetString("port")
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port: %v", err)
		}

		verbose, _ := cmd.Flags().GetBool("verbose")
		accountType, _ := cmd.Flags().GetString("account-type")
		manual, _ := cmd.Flags().GetBool("manual")
		rateLimitStr, _ := cmd.Flags().GetString("rate-limit")
		wait, _ := cmd.Flags().GetBool("wait")
		githubToken, _ := cmd.Flags().GetString("github-token")
		claudeCode, _ := cmd.Flags().GetBool("claude-code")
		console, _ := cmd.Flags().GetBool("console")
		showToken, _ := cmd.Flags().GetBool("show-token")
		proxyEnv, _ := cmd.Flags().GetBool("proxy-env")
		apiKey, _ := cmd.Flags().GetString("api-key")
provider, _ := cmd.Flags().GetString("provider")
	allowLocalEndpoints, _ := cmd.Flags().GetBool("allow-local-endpoints")
	enableConfigEdit, _ := cmd.Flags().GetBool("enable-config-edit")

		var rateLimit *int
		if rateLimitStr != "" {
			rl, err := strconv.Atoi(rateLimitStr)
			if err != nil {
				return fmt.Errorf("invalid rate-limit: %v", err)
			}
			rateLimit = &rl
		}

		options := server.StartOptions{
			Port:                port,
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
	StartCmd.Flags().StringP("port", "p", "5005", "Port to listen on")
	StartCmd.Flags().BoolP("verbose", "v", false, "Enable verbose logging")
	StartCmd.Flags().StringP("account-type", "a", "individual", "Account type to use (individual, business, enterprise)")
	StartCmd.Flags().Bool("manual", false, "Enable manual request approval")
	StartCmd.Flags().StringP("rate-limit", "r", "", "Rate limit in seconds between requests")
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