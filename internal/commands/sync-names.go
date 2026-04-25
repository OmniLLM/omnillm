package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	antigravitypkg "omnillm/internal/providers/antigravity"
	"omnillm/internal/database"
	ghservice "omnillm/internal/services/github"

	"github.com/spf13/cobra"
)

// SyncNamesCmd refreshes every provider instance's display name in the database
// by calling each provider's live API, so names immediately reflect in the UI
// without requiring a re-authentication.
var SyncNamesCmd = &cobra.Command{
	Use:   "sync-names",
	Short: "Refresh provider display names in the database from live API metadata",
	Long: `Calls each provider's user-info API to fetch the most up-to-date
identifiable metadata (email, real name, login) and updates the display name
stored in the database.  Changes take effect immediately in the UI — no server
restart required.

Supported providers: github-copilot, antigravity`,
	RunE: func(cmd *cobra.Command, args []string) error {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir := filepath.Join(homeDir, ".config", "omnillm")

		if err := database.InitializeDatabase(configDir); err != nil {
			return fmt.Errorf("failed to initialise database: %w", err)
		}

		instanceStore := database.NewProviderInstanceStore()
		tokenStore := database.NewTokenStore()

		instances, err := instanceStore.GetAll()
		if err != nil {
			return fmt.Errorf("failed to list provider instances: %w", err)
		}

		updated, skipped, failed := 0, 0, 0

		for _, inst := range instances {
			rec, err := tokenStore.Get(inst.InstanceID)
			if err != nil || rec == nil {
				fmt.Printf("  %-40s  skipped (no token record)\n", inst.InstanceID)
				skipped++
				continue
			}

			var td map[string]interface{}
			if err := json.Unmarshal([]byte(rec.TokenData), &td); err != nil {
				fmt.Printf("  %-40s  skipped (token parse error: %v)\n", inst.InstanceID, err)
				skipped++
				continue
			}

			var newName string

			switch inst.ProviderID {

			case "github-copilot":
				githubToken, _ := td["github_token"].(string)
				if githubToken == "" {
					fmt.Printf("  %-40s  skipped (no github_token)\n", inst.InstanceID)
					skipped++
					continue
				}
				user, err := ghservice.GetUser(githubToken)
				if err != nil {
					fmt.Printf("  %-40s  FAILED  (%v)\n", inst.InstanceID, err)
					failed++
					continue
				}
				newName = ghservice.CopilotProviderName(user)
				// Persist name into token_data so LoadFromDB restores it on restart.
				td["name"] = newName
				_ = tokenStore.Save(inst.InstanceID, inst.ProviderID, td)

			case "antigravity":
				// Try stored email first; otherwise refresh the token and call userinfo.
				email, _ := td["email"].(string)
				if email == "" {
					accessToken, _ := td["access_token"].(string)
					// Attempt a token refresh if we have credentials.
					if rt, ok := td["refresh_token"].(string); ok && rt != "" {
						if cid, ok2 := td["client_id"].(string); ok2 {
							if cs, ok3 := td["client_secret"].(string); ok3 {
								if refreshed, err := antigravitypkg.RefreshAccessToken(cid, cs, rt); err == nil {
									accessToken = refreshed.AccessToken
									td["access_token"] = accessToken
								}
							}
						}
					}
					if accessToken != "" {
						email = antigravitypkg.FetchUserEmail(accessToken)
						if email != "" {
							td["email"] = email
							_ = tokenStore.Save(inst.InstanceID, inst.ProviderID, td)
						}
					}
				}
				if email == "" {
					fmt.Printf("  %-40s  skipped (could not determine email)\n", inst.InstanceID)
					skipped++
					continue
				}
				newName = fmt.Sprintf("Antigravity (%s)", email)

			default:
				// Provider type not handled by this command.
				skipped++
				continue
			}

			if newName == "" || newName == inst.Name {
				fmt.Printf("  %-40s  unchanged  %q\n", inst.InstanceID, inst.Name)
				skipped++
				continue
			}

			oldName := inst.Name
			inst.Name = newName
			if err := instanceStore.Save(&inst); err != nil {
				fmt.Printf("  %-40s  FAILED  (db save: %v)\n", inst.InstanceID, err)
				failed++
				continue
			}

			fmt.Printf("  %-40s  %q  →  %q\n", inst.InstanceID, oldName, newName)
			updated++
		}

		fmt.Println()
		fmt.Printf("Done — %d updated, %d unchanged/skipped, %d failed.\n", updated, skipped, failed)
		return nil
	},
}
