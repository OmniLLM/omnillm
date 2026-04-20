package commands

import (
	"fmt"
	"omnillm/internal/database"
	"omnillm/internal/providers/copilot"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	ghservice "omnillm/internal/services/github"
)

var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with GitHub Copilot",
	Long:  "Authenticate with GitHub Copilot using the device code flow",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize database (needed for token storage)
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir := filepath.Join(homeDir, ".config", "omnillm")
		if err := database.InitializeDatabase(configDir); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}

		fmt.Println("Authenticating with GitHub Copilot...")
		fmt.Println()

		// Step 1: Get device code
		deviceCode, err := ghservice.GetDeviceCode()
		if err != nil {
			return fmt.Errorf("failed to get device code: %w", err)
		}

		fmt.Printf("Please visit: %s\n", deviceCode.VerificationURI)
		fmt.Printf("And enter code: %s\n", deviceCode.UserCode)
		fmt.Println()
		fmt.Println("Waiting for authorization...")

		// Step 2: Poll for access token
		accessToken, err := ghservice.PollAccessToken(deviceCode)
		if err != nil {
			return fmt.Errorf("authorization failed: %w", err)
		}

		fmt.Println("Authorization successful!")
		fmt.Println()

		// Step 3: Get user info
		user, err := ghservice.GetUser(accessToken)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get user info")
		} else {
			login, _ := user["login"].(string)
			fmt.Printf("Authenticated as: %s\n", login)
		}

		// Step 4: Get Copilot token to verify access
		copilotToken, err := ghservice.GetCopilotToken(accessToken)
		if err != nil {
			return fmt.Errorf("failed to get Copilot token (do you have Copilot access?): %w", err)
		}

		fmt.Println("Copilot access verified!")

		// Step 5: Save token to database
		tokenStore := database.NewTokenStore()
		tokenData := map[string]interface{}{
			"github_token":  accessToken,
			"copilot_token": copilotToken.Token,
			"expires_at":    copilotToken.ExpiresAt,
		}
		if user != nil {
			if login, ok := user["login"].(string); ok {
				tokenData["username"] = login
			}
		}

		instanceID := "github-copilot-1"
		if user != nil {
			if login, ok := user["login"].(string); ok {
				instanceID = fmt.Sprintf("github-copilot-%s", login)
			}
		}

		if err := tokenStore.Save(instanceID, "github-copilot", tokenData); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}

		// Register provider in registry
		providerRegistry := registry.GetProviderRegistry()
		copilotProvider := copilot.NewGitHubCopilotProvider(instanceID)
		if err := copilotProvider.SetupAuth(&types.AuthOptions{GithubToken: copilotToken.Token}); err != nil {
			return fmt.Errorf("failed to configure provider auth: %w", err)
		}
		if err := providerRegistry.Register(copilotProvider, true); err != nil {
			return fmt.Errorf("failed to register provider: %w", err)
		}
		if _, err := providerRegistry.SetActive(instanceID); err != nil {
			return fmt.Errorf("failed to activate provider: %w", err)
		}

		fmt.Println()
		fmt.Printf("Provider '%s' is ready. Start the server with:\n", instanceID)
		fmt.Println("  omnillm start")
		fmt.Println()

		return nil
	},
}
