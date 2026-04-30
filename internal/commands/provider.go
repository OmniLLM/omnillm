package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var ProviderCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage LLM providers",
	Long:  "List, add, remove, and configure LLM provider instances.",
}

func init() {
	// provider list
	ProviderCmd.AddCommand(providerListCmd)

	// provider add
	providerAddCmd.Flags().String("api-key", "", "API key for the provider")
	providerAddCmd.Flags().String("token", "", "GitHub token (github-copilot)")
	providerAddCmd.Flags().String("endpoint", "", "Base URL endpoint (openai-compatible)")
	providerAddCmd.Flags().String("region", "", "Region (alibaba, azure-openai)")
	providerAddCmd.Flags().String("plan", "", "Plan (alibaba: standard|coding-plan)")
	providerAddCmd.Flags().BoolP("yes", "y", false, "Skip confirmations")
	ProviderCmd.AddCommand(providerAddCmd)

	// provider delete
	providerDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	ProviderCmd.AddCommand(providerDeleteCmd)

	// provider activate / deactivate / switch
	ProviderCmd.AddCommand(providerActivateCmd)
	ProviderCmd.AddCommand(providerDeactivateCmd)
	ProviderCmd.AddCommand(providerSwitchCmd)

	// provider rename
	providerRenameCmd.Flags().String("name", "", "New display name")
	providerRenameCmd.Flags().String("subtitle", "", "New subtitle")
	ProviderCmd.AddCommand(providerRenameCmd)

	// provider priorities
	providerPrioritiesCmd.Flags().StringSlice("set", nil, "Set priorities: id:N,... (repeatable)")
	ProviderCmd.AddCommand(providerPrioritiesCmd)

	// provider usage
	ProviderCmd.AddCommand(providerUsageCmd)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// parseProviders unmarshals provider list from JSON response.
func parseProviders(data []byte) ([]map[string]interface{}, error) {
	var providers []map[string]interface{}
	if err := json.Unmarshal(data, &providers); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return providers, nil
}

// getStringFlag is a helper to get a string flag value with error handling.
func getStringFlag(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// getStringSliceFlag is a helper to get a string slice flag value with error handling.
func getStringSliceFlag(cmd *cobra.Command, name string) []string {
	v, _ := cmd.Flags().GetStringSlice(name)
	return v
}

// getBoolFlag is a helper to get a bool flag value with error handling.
func getBoolFlag(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

// ─── list ─────────────────────────────────────────────────────────────────────

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/providers")
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		providers, err := parseProviders(data)
		if err != nil {
			return err
		}

		if len(providers) == 0 {
			fmt.Println("No providers configured.")
			return nil
		}

		fmt.Printf("%-36s  %-20s  %-36s  %-15s  %-8s  %s\n",
			"ID", "TYPE", "NAME", "AUTH", "ACTIVE", "MODELS")
		fmt.Println(strings.Repeat("─", 130))

		for _, p := range providers {
			id, _ := p["id"].(string)
			pType, _ := p["type"].(string)
			name, _ := p["name"].(string)
			auth, _ := p["authStatus"].(string)

			active := "no"
			if isActive, ok := p["isActive"].(bool); ok && isActive {
				active = "yes"
			}

			total, _ := p["totalModelCount"].(float64)
			enabled, _ := p["enabledModelCount"].(float64)
			models := fmt.Sprintf("%d/%d", int(enabled), int(total))

			fmt.Printf("%-36s  %-20s  %-36s  %-15s  %-8s  %s\n",
				padRight(id, 36), padRight(pType, 20), padRight(name, 36),
				padRight(auth, 15), padRight(active, 8), models)
		}
		return nil
	},
}

// ─── add ──────────────────────────────────────────────────────────────────────

var providerAddCmd = &cobra.Command{
	Use:   "add <type>",
	Short: "Add and authenticate a new provider instance",
	Long: `Add a new provider instance. Supported types:
  github-copilot    GitHub Copilot (device-code OAuth or --token)
  openai-compatible Any OpenAI-compatible API (requires --endpoint and --api-key)
  alibaba           Alibaba DashScope (requires --api-key; optional --region, --plan)
  azure-openai      Azure OpenAI (requires --api-key)
  google            Google AI (requires --api-key)
  kimi              Kimi AI (requires --api-key)
  codex             OpenAI Codex (requires --api-key)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		providerType := args[0]

		body := map[string]interface{}{
			"api_key":  getStringFlag(cmd, "api-key"),
			"apiKey":   getStringFlag(cmd, "api-key"),
			"token":    getStringFlag(cmd, "token"),
			"endpoint": getStringFlag(cmd, "endpoint"),
			"region":   getStringFlag(cmd, "region"),
			"plan":     getStringFlag(cmd, "plan"),
		}

		data, err := c.Post("/api/admin/providers/auth-and-create/"+providerType, body)
		if err != nil {
			return err
		}

		var resp map[string]interface{}
		if err := c.parseJSON(data, &resp); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		// Handle device-code OAuth flow
		if requiresAuth, ok := resp["requiresAuth"].(bool); ok && requiresAuth {
			return handleAuthFlow(c, &resp)
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		success, _ := resp["success"].(bool)
		if !success {
			msg, _ := resp["message"].(string)
			return fmt.Errorf("failed: %s", msg)
		}

		if prov, ok := resp["provider"].(map[string]interface{}); ok {
			id, _ := prov["id"].(string)
			name, _ := prov["name"].(string)
			SuccessMsg("Provider '%s' (%s) added successfully.", id, name)
		}
		return nil
	},
}

// handleAuthFlow polls for authentication completion.
func handleAuthFlow(c *Client, resp *map[string]interface{}) error {
	verifyURI, _ := (*resp)["verification_uri"].(string)
	userCode, _ := (*resp)["user_code"].(string)
	fmt.Printf("\n  Visit: %s\n  Code:  %s\n\nWaiting for authorization", verifyURI, userCode)

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

		status, _ := statusResp["status"].(string)
		switch status {
		case "complete":
			fmt.Println()
			providerID, _ := statusResp["providerId"].(string)
			SuccessMsg("Provider '%s' authenticated successfully.", providerID)
			return nil
		case "error":
			fmt.Println()
			errMsg, _ := statusResp["error"].(string)
			return fmt.Errorf("authentication failed: %s", errMsg)
		}
	}
}

// ─── delete ───────────────────────────────────────────────────────────────────

var providerDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a provider instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if !getBoolFlag(cmd, "yes") && !Confirm(fmt.Sprintf("Delete provider '%s'?", id)) {
			fmt.Println("Cancelled.")
			return nil
		}

		c := NewClient(cmd)
		data, err := c.Delete("/api/admin/providers/" + id)
		if err != nil {
			return err
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		SuccessMsg("Provider '%s' deleted.", id)
		return nil
	},
}

// ─── activate ─────────────────────────────────────────────────────────────────

var providerActivateCmd = &cobra.Command{
	Use:   "activate <id>",
	Short: "Activate a provider (add to active set)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Post("/api/admin/providers/"+args[0]+"/activate", nil)
		if err != nil {
			return err
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		SuccessMsg("Provider '%s' activated.", args[0])
		return nil
	},
}

// ─── deactivate ───────────────────────────────────────────────────────────────

var providerDeactivateCmd = &cobra.Command{
	Use:   "deactivate <id>",
	Short: "Deactivate a provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Post("/api/admin/providers/"+args[0]+"/deactivate", nil)
		if err != nil {
			return err
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		SuccessMsg("Provider '%s' deactivated.", args[0])
		return nil
	},
}

// ─── switch ───────────────────────────────────────────────────────────────────

var providerSwitchCmd = &cobra.Command{
	Use:   "switch <id>",
	Short: "Switch the primary active provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Post("/api/admin/providers/switch", map[string]string{"providerId": args[0]})
		if err != nil {
			return err
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		SuccessMsg("Switched active provider to '%s'.", args[0])
		return nil
	},
}

// ─── rename ───────────────────────────────────────────────────────────────────

var providerRenameCmd = &cobra.Command{
	Use:   "rename <id>",
	Short: "Rename a provider or update its subtitle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := getStringFlag(cmd, "name")
		subtitle := getStringFlag(cmd, "subtitle")

		if name == "" && subtitle == "" {
			return fmt.Errorf("at least one of --name or --subtitle is required")
		}

		body := map[string]interface{}{}
		if name != "" {
			body["name"] = name
		}
		if subtitle != "" {
			body["subtitle"] = subtitle
		}

		c := NewClient(cmd)
		data, err := c.Patch("/api/admin/providers/"+args[0]+"/name", body)
		if err != nil {
			return err
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		SuccessMsg("Provider '%s' renamed.", args[0])
		return nil
	},
}

// ─── priorities ───────────────────────────────────────────────────────────────

var providerPrioritiesCmd = &cobra.Command{
	Use:   "priorities",
	Short: "Get or set provider priorities",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		sets := getStringSliceFlag(cmd, "set")

		if len(sets) == 0 {
			// GET
			data, err := c.Get("/api/admin/providers/priorities")
			if err != nil {
				return err
			}
			if c.IsJSON() {
				c.PrintJSON(data)
				return nil
			}

			var resp map[string]interface{}
			if err := c.parseJSON(data, &resp); err != nil {
				return err
			}

			priorities, _ := resp["priorities"].(map[string]interface{})
			fmt.Printf("%-40s  %s\n", "PROVIDER ID", "PRIORITY")
			fmt.Println(strings.Repeat("─", 50))

			for id, p := range priorities {
				fmt.Printf("%-40s  %.0f\n", id, p)
			}
			return nil
		}

		// POST — parse "id:N" pairs
		priorities := map[string]int{}
		for _, s := range sets {
			parts := strings.SplitN(s, ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --set value %q: expected id:N", s)
			}

			var n int
			if _, err := fmt.Sscanf(parts[1], "%d", &n); err != nil {
				return fmt.Errorf("invalid priority %q: %w", parts[1], err)
			}
			priorities[parts[0]] = n
		}

		body := map[string]interface{}{"priorities": priorities}
		data, err := c.Post("/api/admin/providers/priorities", body)
		if err != nil {
			return err
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		SuccessMsg("Provider priorities updated.")
		return nil
	},
}

// ─── usage ────────────────────────────────────────────────────────────────────

var providerUsageCmd = &cobra.Command{
	Use:   "usage <id>",
	Short: "Show usage/quota for a provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/providers/" + args[0] + "/usage")
		if err != nil {
			return err
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		var usage map[string]interface{}
		if err := json.Unmarshal(data, &usage); err != nil {
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("Usage for %s:\n", args[0])
		fmt.Println(strings.Repeat("─", 40))

		for k, v := range usage {
			fmt.Printf("  %-24s %v\n", k+":", v)
		}
		return nil
	},
}
