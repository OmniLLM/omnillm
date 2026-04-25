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
	// Add subcommands
	rootCmd.AddCommand(commands.AuthCmd)
	rootCmd.AddCommand(commands.StartCmd)
	rootCmd.AddCommand(commands.CheckUsageCmd)
	rootCmd.AddCommand(commands.DebugCmd)
	rootCmd.AddCommand(commands.ChatCmd)
	rootCmd.AddCommand(commands.SyncNamesCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
