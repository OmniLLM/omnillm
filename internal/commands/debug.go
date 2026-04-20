package commands

import (
	"fmt"
	"omnillm/internal/database"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var DebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Print debug information about the runtime and configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("OmniLLM Debug Info")
		fmt.Println("════════════════════")
		fmt.Println()

		// Runtime info
		fmt.Println("Runtime:")
		fmt.Printf("  Go version: %s\n", runtime.Version())
		fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("  NumCPU:     %d\n", runtime.NumCPU())
		fmt.Println()

		// Paths
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir := filepath.Join(homeDir, ".config", "omnillm")
		dbPath := filepath.Join(configDir, "database.sqlite")

		fmt.Println("Paths:")
		fmt.Printf("  Home:     %s\n", homeDir)
		fmt.Printf("  Config:   %s\n", configDir)
		fmt.Printf("  Database: %s\n", dbPath)

		// Check if database exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Println("  Database: NOT FOUND")
		} else {
			info, _ := os.Stat(dbPath)
			fmt.Printf("  DB Size:  %d bytes\n", info.Size())
		}
		fmt.Println()

		// Token status
		if err := database.InitializeDatabase(configDir); err != nil {
			fmt.Printf("  Database error: %v\n", err)
			return nil
		}

		tokenStore := database.NewTokenStore()
		tokens, err := tokenStore.GetAllByProvider("github-copilot")
		if err != nil {
			fmt.Printf("  Token error: %v\n", err)
		} else {
			fmt.Println("Tokens:")
			if len(tokens) == 0 {
				fmt.Println("  No tokens stored. Run 'omnillm auth' to authenticate.")
			}
			for _, t := range tokens {
				fmt.Printf("  Instance: %s (provider: %s)\n", t.InstanceID, t.ProviderID)
			}
		}
		fmt.Println()

		// Provider instances
		instanceStore := database.NewProviderInstanceStore()
		instances, err := instanceStore.GetAll()
		if err != nil {
			fmt.Printf("  Instance error: %v\n", err)
		} else {
			fmt.Println("Provider Instances:")
			if len(instances) == 0 {
				fmt.Println("  No provider instances stored.")
			}
			for _, inst := range instances {
				activated := "inactive"
				if inst.Activated {
					activated = "active"
				}
				fmt.Printf("  %s [%s] priority=%d (%s)\n", inst.InstanceID, inst.ProviderID, inst.Priority, activated)
			}
		}

		return nil
	},
}
