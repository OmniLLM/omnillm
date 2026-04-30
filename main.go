package main

import (
	"fmt"
	"omnillm/internal/commands"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "omnillm",
	Short:         "Universal LLM proxy with OpenAI-compatible endpoints",
	Long:          "OmniLLM is a universal LLM proxy that provides OpenAI-compatible endpoints for multiple upstream providers.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func main() {
	// Persistent flags available to all commands
	rootCmd.PersistentFlags().String("server", "http://127.0.0.1:5000",
		"OmniLLM server address (or set OMNILLM_SERVER)")
	rootCmd.PersistentFlags().String("api-key", "",
		"Admin API key for the server (or set OMNILLM_API_KEY)")
	rootCmd.PersistentFlags().StringP("output", "o", "table",
		"Output format: table or json")

	// Server lifecycle
	rootCmd.AddCommand(commands.StartCmd)

	// Provider/backend management helpers
	rootCmd.AddCommand(commands.AuthCmd)
	rootCmd.AddCommand(commands.UsageCmd)
	rootCmd.AddCommand(commands.CheckUsageCmd)
	rootCmd.AddCommand(commands.SyncNamesCmd)
	rootCmd.AddCommand(commands.DebugCmd)

	// Admin API commands (require running server)
	rootCmd.AddCommand(commands.ProviderCmd)
	rootCmd.AddCommand(commands.ModelCmd)
	rootCmd.AddCommand(commands.VirtualModelCmd)
	rootCmd.AddCommand(commands.ConfigCmd)
	rootCmd.AddCommand(commands.SettingsCmd)
	rootCmd.AddCommand(commands.StatusCmd)
	rootCmd.AddCommand(commands.ChatCmd)
	rootCmd.AddCommand(commands.LogsCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
