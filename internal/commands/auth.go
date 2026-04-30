package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	ghservice "omnillm/internal/services/github"
)

var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with GitHub Copilot",
	Long:  "Authenticate with GitHub Copilot using the device code flow",
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// Step 3: Get user info and Copilot token
		user, err := ghservice.GetUser(accessToken)
		if err == nil {
			if login, ok := user["login"].(string); ok {
				fmt.Printf("Authenticated as: %s\n", login)
			}
		}

		_, err = ghservice.GetCopilotToken(accessToken)
		if err != nil {
			return fmt.Errorf("failed to get Copilot token (do you have Copilot access?): %w", err)
		}

		fmt.Println("Copilot access verified!")
		fmt.Println()

		// Step 4: Register provider via backend
		c := NewClient(cmd)
		body := map[string]interface{}{
			"token": accessToken,
		}

		data, err := c.Post("/api/admin/providers/auth-and-create/github-copilot", body)
		if err != nil {
			return err
		}

		// Handle device-code OAuth flow if needed
		var resp map[string]interface{}
		if err := c.parseJSON(data, &resp); err != nil {
			return err
		}

		if requiresAuth, ok := resp["requiresAuth"].(bool); ok && requiresAuth {
			verifyURI, _ := resp["verification_uri"].(string)
			userCode, _ := resp["user_code"].(string)
			fmt.Printf("\n  Visit: %s\n  Code:  %s\n\nWaiting for authorization", verifyURI, userCode)

			// Poll auth-status until complete
			for {
				time.Sleep(3 * time.Second)
				fmt.Print(".")

				statusData, err := c.Get("/api/admin/auth-status")
				if err != nil {
					continue
				}
				var statusResp map[string]interface{}
				if err := c.parseJSON(statusData, &statusResp); err != nil {
					continue
				}

				switch status, _ := statusResp["status"].(string); status {
				case "complete":
					fmt.Println()
					if providerID, ok := statusResp["providerId"].(string); ok {
						SuccessMsg("Provider '%s' authenticated successfully.", providerID)
					}
					return nil
				case "error":
					fmt.Println()
					if errMsg, ok := statusResp["error"].(string); ok {
						return fmt.Errorf("authentication failed: %s", errMsg)
					}
					return fmt.Errorf("authentication failed")
				}
			}
		}

		// Provider created successfully
		if prov, ok := resp["provider"].(map[string]interface{}); ok {
			if id, ok := prov["id"].(string); ok {
				SuccessMsg("Provider '%s' created and authenticated.", id)
			}
		}

		return nil
	},
}
