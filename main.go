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
	configureRootCommand()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func configureRootCommand() {
	// Persistent flags
	rootCmd.PersistentFlags().String("server", "http://127.0.0.1:5000",
		"OmniLLM server address (or set OMNILLM_SERVER)")
	rootCmd.PersistentFlags().String("api-key", "",
		"Admin API key for the server (or set OMNILLM_API_KEY)")
	rootCmd.PersistentFlags().StringP("output", "o", "table",
		"Output format: table or json")

	// --output flag completion
	_ = rootCmd.RegisterFlagCompletionFunc("output", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"table", "json"}, cobra.ShellCompDirectiveNoFileComp
	})

	// Command groups
	rootCmd.AddGroup(&cobra.Group{ID: "server", Title: "Server:"})
	rootCmd.AddGroup(&cobra.Group{ID: "providers", Title: "Providers:"})
	rootCmd.AddGroup(&cobra.Group{ID: "admin", Title: "Admin:"})
	rootCmd.AddGroup(&cobra.Group{ID: "troubleshoot", Title: "Troubleshooting:"})

	// Server
	commands.StartCmd.GroupID = "server"
	rootCmd.AddCommand(commands.StartCmd)

	// Providers
	commands.AuthCmd.GroupID = "providers"
	commands.ProviderCmd.GroupID = "providers"
	commands.LegacyModelCmd.GroupID = "providers"
	commands.VirtualModelCmd.GroupID = "providers"
	rootCmd.AddCommand(commands.AuthCmd)
	rootCmd.AddCommand(commands.ProviderCmd)
	rootCmd.AddCommand(commands.LegacyModelCmd)
	rootCmd.AddCommand(commands.VirtualModelCmd)

	// Admin
	commands.ConfigCmd.GroupID = "admin"
	commands.SettingsCmd.GroupID = "admin"
	commands.StatusCmd.GroupID = "admin"
	commands.LogsCmd.GroupID = "admin"
	commands.UsageCmd.GroupID = "admin"
	rootCmd.AddCommand(commands.ConfigCmd)
	rootCmd.AddCommand(commands.SettingsCmd)
	rootCmd.AddCommand(commands.StatusCmd)
	rootCmd.AddCommand(commands.LogsCmd)
	rootCmd.AddCommand(commands.UsageCmd)
	rootCmd.AddCommand(commands.CheckUsageCmd)
	rootCmd.AddCommand(commands.SyncNamesCmd)

	// Troubleshooting
	commands.DoctorCmd.GroupID = "troubleshoot"
	commands.DebugCmd.GroupID = "troubleshoot"
	rootCmd.AddCommand(commands.DoctorCmd)
	rootCmd.AddCommand(commands.DebugCmd)

	// Completions
	rootCmd.AddCommand(commands.CompletionCmd)

}
