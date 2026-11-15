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
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		fmt.Fprintln(out, "OmniLLM Debug Info")
		fmt.Fprintln(out, "════════════════════")
		fmt.Fprintln(out)

		// Runtime info
		fmt.Fprintln(out, "Runtime:")
		fmt.Fprintf(out, "  Go version: %s\n", runtime.Version())
		fmt.Fprintf(out, "  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Fprintf(out, "  NumCPU:     %d\n", runtime.NumCPU())
		fmt.Fprintln(out)

		// Paths
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir := filepath.Join(homeDir, ".config", "omnillm")
		dbPath := filepath.Join(configDir, "database.sqlite")

		fmt.Fprintln(out, "Paths:")
		fmt.Fprintf(out, "  Home:     %s\n", homeDir)
		fmt.Fprintf(out, "  Config:   %s\n", configDir)
		fmt.Fprintf(out, "  Database: %s\n", dbPath)

		// Avoid creating a database file when inspecting an uninitialized machine.
		info, err := os.Stat(dbPath)
		if os.IsNotExist(err) {
			fmt.Fprintln(out, "  Database: NOT FOUND")
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Tokens:")
			fmt.Fprintln(out, "  No tokens stored. Run 'omnillm provider login' to authenticate.")
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Provider Instances:")
			fmt.Fprintln(out, "  No provider instances stored.")
			return nil
		}
		if err != nil {
			fmt.Fprintf(out, "  Database error: %v\n", err)
			return nil
		}
		fmt.Fprintf(out, "  DB Size:  %d bytes\n", info.Size())
		fmt.Fprintln(out)

		if err := database.InitializeDatabase(configDir); err != nil {
			fmt.Fprintf(out, "  Database error: %v\n", err)
			return nil
		}
		defer func() {
			database.StopAsyncWorkers()
			_ = database.GetDatabase().Close()
		}()

		instanceStore := database.NewProviderInstanceStore()
		instances, instErr := instanceStore.GetAll()
		tokenStore := database.NewTokenStore()

		fmt.Fprintln(out, "Tokens:")
		if instErr != nil {
			fmt.Fprintf(out, "  Token error: %v\n", instErr)
		} else {
			tokenCount := 0
			for _, inst := range instances {
				rec, tokenErr := tokenStore.Get(inst.InstanceID)
				if tokenErr == nil && rec != nil {
					fmt.Fprintf(out, "  Instance: %s [%s]\n", rec.InstanceID, inst.ProviderID)
					tokenCount++
				}
			}
			if tokenCount == 0 {
				fmt.Fprintln(out, "  No tokens stored. Run 'omnillm provider login' to authenticate.")
			}
		}
		fmt.Fprintln(out)

		fmt.Fprintln(out, "Provider Instances:")
		if instErr != nil {
			fmt.Fprintf(out, "  Instance error: %v\n", instErr)
		} else {
			if len(instances) == 0 {
				fmt.Fprintln(out, "  No provider instances stored.")
			}
			for _, inst := range instances {
				activated := "inactive"
				if inst.Activated {
					activated = "active"
				}
				fmt.Fprintf(out, "  %s [%s] priority=%d (%s)\n", inst.InstanceID, inst.ProviderID, inst.Priority, activated)
			}
		}

		return nil
	},
}
