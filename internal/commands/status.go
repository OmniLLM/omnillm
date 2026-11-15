package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show server status",
	Args:  cobra.NoArgs,
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

	out := cmd.OutOrStdout()
	status, _ := resp["status"].(string)
	uptime, _ := resp["uptime"].(string)
	modelCount, _ := resp["modelCount"].(float64)
	manualApprove, _ := resp["manualApprove"].(bool)
	rateLimitSeconds := resp["rateLimitSeconds"]
	rateLimitWait, _ := resp["rateLimitWait"].(bool)

	manualApproveStr := "no"
	if manualApprove {
		manualApproveStr = "yes"
	}
	rateLimitStr := "none"
	if rateLimitSeconds != nil {
		rateLimitStr = fmt.Sprintf("%vs (wait=%v)", rateLimitSeconds, rateLimitWait)
	}

	if err := PrintSection(out, "Server status"); err != nil {
		return err
	}
	summaryTable := NewTable("FIELD", "VALUE")
	summaryTable.AddRow("Status", status)
	summaryTable.AddRow("Uptime", uptime)
	summaryTable.AddRow("Model count", fmt.Sprintf("%.0f", modelCount))
	summaryTable.AddRow("Manual approve", manualApproveStr)
	summaryTable.AddRow("Rate limit", rateLimitStr)
	if err := summaryTable.Render(out); err != nil {
		return err
	}

	providerCount := 0
	providerTable := NewTable("Name", "ID")
	if providers, ok := resp["activeProviders"].([]interface{}); ok {
		for _, entry := range providers {
			provider, _ := entry.(map[string]interface{})
			name, _ := provider["name"].(string)
			id, _ := provider["id"].(string)
			if name == "" && id == "" {
				continue
			}
			providerTable.AddRow(name, id)
			providerCount++
		}
	} else if ap, ok := resp["activeProvider"].(map[string]interface{}); ok {
		name, _ := ap["name"].(string)
		id, _ := ap["id"].(string)
		if name != "" || id != "" {
			providerTable.AddRow(name, id)
			providerCount = 1
		}
	}

	if providerCount > 0 {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if err := PrintSection(out, "Active providers"); err != nil {
			return err
		}
		if err := providerTable.Render(out); err != nil {
			return err
		}
	} else if err := PrintKeyValue(out, "Active providers", "none"); err != nil {
		return err
	}

	if services, ok := resp["services"].(map[string]interface{}); ok {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if err := PrintSection(out, "Services"); err != nil {
			return err
		}
		serviceTable := NewTable("Service", "Status")
		serviceTable.AddRow("API", fmt.Sprint(services["api"]))
		serviceTable.AddRow("Database", fmt.Sprint(services["database"]))
		if providers, ok := services["providers"].(map[string]interface{}); ok {
			serviceTable.AddRow("Providers", fmt.Sprintf("%v total, %v active", providers["total"], providers["active"]))
		}
		if err := serviceTable.Render(out); err != nil {
			return err
		}
	}

	if authFlow, ok := resp["authFlow"].(map[string]interface{}); ok && authFlow != nil {
		flowStatus, _ := authFlow["status"].(string)
		providerID, _ := authFlow["providerId"].(string)
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if err := PrintSection(out, "Active auth flow"); err != nil {
			return err
		}
		authTable := NewTable("Field", "Value")
		authTable.AddRow("Status", flowStatus)
		authTable.AddRow("Provider", providerID)
		if uc, ok := authFlow["userCode"].(string); ok && uc != "" {
			authTable.AddRow("User code", uc)
		}
		if url, ok := authFlow["instructionURL"].(string); ok && url != "" {
			authTable.AddRow("Visit", url)
		}
		if err := authTable.Render(out); err != nil {
			return err
		}
	}

	return nil
}

var statusAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Show active authentication flow status",
	Args:  cobra.NoArgs,
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
			return PrintEmpty(cmd.OutOrStdout(), "active authentication flow")
		}

		out := cmd.OutOrStdout()
		providerID, _ := resp["providerId"].(string)
		if err := PrintSection(out, "Auth flow status"); err != nil {
			return err
		}
		table := NewTable("Field", "Value")
		table.AddRow("Provider", providerID)
		table.AddRow("Status", status)
		if uc, ok := resp["userCode"].(string); ok && uc != "" {
			table.AddRow("User code", uc)
		}
		if url, ok := resp["instructionURL"].(string); ok && url != "" {
			table.AddRow("Visit", url)
		}
		if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
			table.AddRow("Error", errMsg)
		}
		return table.Render(out)
	},
}
