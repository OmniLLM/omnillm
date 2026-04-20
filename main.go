package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"omnillm/internal/commands"
)

var rootCmd = &cobra.Command{
	Use:   "omnillm",
	Short: "A wrapper around GitHub Copilot API to make it OpenAI compatible",
	Long:  "A wrapper around GitHub Copilot API to make it OpenAI compatible, making it usable for other tools.",
}

func main() {
	// Add subcommands
	rootCmd.AddCommand(commands.AuthCmd)
	rootCmd.AddCommand(commands.StartCmd)
	rootCmd.AddCommand(commands.CheckUsageCmd)
	rootCmd.AddCommand(commands.DebugCmd)
	rootCmd.AddCommand(commands.ChatCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}