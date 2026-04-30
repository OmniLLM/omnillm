package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var ModelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage models for a provider",
}

func init() {
	ModelCmd.AddCommand(modelListCmd)
	ModelCmd.AddCommand(modelRefreshCmd)

	modelToggleCmd.Flags().Bool("enable", false, "Enable the model")
	modelToggleCmd.Flags().Bool("disable", false, "Disable the model")
	ModelCmd.AddCommand(modelToggleCmd)

	modelVersionCmd := &cobra.Command{
		Use:   "version",
		Short: "Get or set a model version",
	}
	modelVersionCmd.AddCommand(modelVersionGetCmd)
	modelVersionCmd.AddCommand(modelVersionSetCmd)
	ModelCmd.AddCommand(modelVersionCmd)
}

// ─── list ─────────────────────────────────────────────────────────────────────

var modelListCmd = &cobra.Command{
	Use:   "list <provider-id>",
	Short: "List models for a provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/providers/" + args[0] + "/models")
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

		models, _ := resp["models"].([]interface{})
		if len(models) == 0 {
			return PrintEmpty(cmd.OutOrStdout(), "models found")
		}

		table := NewTable("MODEL ID", "NAME", "ENABLED")
		for _, m := range models {
			model, _ := m.(map[string]interface{})
			id, _ := model["id"].(string)
			name, _ := model["name"].(string)
			enabled := "no"
			if v, ok := model["enabled"].(bool); ok && v {
				enabled = "yes"
			}
			table.AddRow(id, name, enabled)
		}
		if err := table.Render(cmd.OutOrStdout()); err != nil {
			return err
		}
		total, _ := resp["total"].(float64)
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "\n%d model(s)\n", int(total))
		return err
	},
}

// ─── refresh ──────────────────────────────────────────────────────────────────

var modelRefreshCmd = &cobra.Command{
	Use:   "refresh <provider-id>",
	Short: "Refresh the model list for a provider from the upstream API",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Post("/api/admin/providers/"+args[0]+"/models/refresh", nil)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		var resp map[string]interface{}
		_ = json.Unmarshal(data, &resp)
		total, _ := resp["total"].(float64)
		SuccessMsg(cmd,"Model list refreshed. %d model(s) available.", int(total))
		return nil
	},
}

// ─── toggle ───────────────────────────────────────────────────────────────────

var modelToggleCmd = &cobra.Command{
	Use:   "toggle <provider-id> <model-id>",
	Short: "Enable or disable a model",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		enable, _ := cmd.Flags().GetBool("enable")
		disable, _ := cmd.Flags().GetBool("disable")
		if !enable && !disable {
			return fmt.Errorf("specify --enable or --disable")
		}
		enabled := enable && !disable

		c := NewClient(cmd)
		body := map[string]interface{}{
			"modelId": args[1],
			"enabled": enabled,
		}
		data, err := c.Post("/api/admin/providers/"+args[0]+"/models/toggle", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		state := "enabled"
		if !enabled {
			state = "disabled"
		}
		SuccessMsg(cmd,"Model '%s' %s.", args[1], state)
		return nil
	},
}

// ─── version get ──────────────────────────────────────────────────────────────

var modelVersionGetCmd = &cobra.Command{
	Use:   "get <provider-id> <model-id>",
	Short: "Get the pinned version for a model",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		path := fmt.Sprintf("/api/admin/providers/%s/models/%s/version", args[0], args[1])
		data, err := c.Get(path)
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
		version, _ := resp["version"].(string)
		if version == "" {
			fmt.Println("No version pinned (using provider default).")
		} else {
			fmt.Printf("Version: %s\n", version)
		}
		return nil
	},
}

// ─── version set ──────────────────────────────────────────────────────────────

var modelVersionSetCmd = &cobra.Command{
	Use:   "set <provider-id> <model-id> <version>",
	Short: "Pin a model to a specific version",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		path := fmt.Sprintf("/api/admin/providers/%s/models/%s/version", args[0], args[1])
		body := map[string]string{"version": args[2]}
		data, err := c.Put(path, body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd,"Model '%s' pinned to version '%s'.", args[1], args[2])
		return nil
	},
}
