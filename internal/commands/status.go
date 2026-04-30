package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show server status",
	RunE:  runServerStatus,
}

func init() {
	StatusCmd.AddCommand(statusAuthCmd)
}

func runServerStatus(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/status")
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}

	status, _ := resp["status"].(string)
	uptime, _ := resp["uptime"].(string)
	modelCount, _ := resp["modelCount"].(float64)
	manualApprove, _ := resp["manualApprove"].(bool)
	rateLimitSeconds := resp["rateLimitSeconds"]
	rateLimitWait, _ := resp["rateLimitWait"].(bool)

	activeProviders := []string{}
	if providers, ok := resp["activeProviders"].([]interface{}); ok {
		for _, entry := range providers {
			provider, _ := entry.(map[string]interface{})
			name, _ := provider["name"].(string)
			id, _ := provider["id"].(string)
			if name != "" && id != "" {
				activeProviders = append(activeProviders, fmt.Sprintf("%s (%s)", name, id))
			}
		}
	}
	activeProvider := "none"
	if len(activeProviders) > 0 {
		activeProvider = strings.Join(activeProviders, ", ")
	} else if ap, ok := resp["activeProvider"].(map[string]interface{}); ok {
		name, _ := ap["name"].(string)
		id, _ := ap["id"].(string)
		activeProvider = fmt.Sprintf("%s (%s)", name, id)
	}

	fmt.Printf("Status:          %s\n", status)
	fmt.Printf("Uptime:          %s\n", uptime)
	fmt.Printf("Active provider: %s\n", activeProvider)
	fmt.Printf("Model count:     %.0f\n", modelCount)
	fmt.Printf("Manual approve:  %v\n", manualApprove)
	if rateLimitSeconds != nil {
		fmt.Printf("Rate limit:      %vs (wait=%v)\n", rateLimitSeconds, rateLimitWait)
	} else {
		fmt.Printf("Rate limit:      none\n")
	}

	if services, ok := resp["services"].(map[string]interface{}); ok {
		fmt.Println("\nServices:")
		fmt.Printf("  API:      %v\n", services["api"])
		fmt.Printf("  Database: %v\n", services["database"])
		if providers, ok := services["providers"].(map[string]interface{}); ok {
			fmt.Printf("  Providers: %v total, %v active\n",
				providers["total"], providers["active"])
		}
	}

	if authFlow, ok := resp["authFlow"].(map[string]interface{}); ok && authFlow != nil {
		flowStatus, _ := authFlow["status"].(string)
		providerID, _ := authFlow["providerId"].(string)
		fmt.Printf("\nActive auth flow: %s (%s)\n", flowStatus, providerID)
		if uc, ok := authFlow["userCode"].(string); ok {
			fmt.Printf("  User code: %s\n", uc)
		}
		if url, ok := authFlow["instructionURL"].(string); ok {
			fmt.Printf("  Visit:     %s\n", url)
		}
	}

	return nil
}

var statusAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Show active authentication flow status",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/auth-status")
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}

		status, _ := resp["status"].(string)
		if status == "idle" {
			fmt.Println("No active authentication flow.")
			return nil
		}

		providerID, _ := resp["providerId"].(string)
		fmt.Printf("Provider:  %s\n", providerID)
		fmt.Printf("Status:    %s\n", status)

		if uc, ok := resp["userCode"].(string); ok && uc != "" {
			fmt.Printf("User code: %s\n", uc)
		}
		if url, ok := resp["instructionURL"].(string); ok && url != "" {
			fmt.Printf("Visit:     %s\n", url)
		}
		if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
			fmt.Printf("Error:     %s\n", errMsg)
		}

		_ = strings.Repeat("", 0) // keep strings import
		return nil
	},
}
