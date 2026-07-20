package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var SettingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "View and update server settings",
}

func init() {
	settingsGetCmd.AddCommand(settingsGetLogLevelCmd)
	settingsGetCmd.AddCommand(settingsGetResponseCacheCmd)
	SettingsCmd.AddCommand(settingsGetCmd)

	settingsSetCmd.AddCommand(settingsSetLogLevelCmd)
	settingsSetLogLevelCmd.ValidArgs = []string{"fatal", "error", "warn", "info", "debug", "trace"}
	settingsSetResponseCacheCmd.Flags().Int("ttl", -1, "TTL in seconds (omit to leave unchanged)")
	settingsSetCmd.AddCommand(settingsSetResponseCacheCmd)
	SettingsCmd.AddCommand(settingsSetCmd)
	SettingsCmd.AddCommand(settingsClearResponseCacheCmd)
}

var settingsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a setting value",
}

var settingsGetLogLevelCmd = &cobra.Command{
	Use:   "log-level",
	Short: "Get the current log level",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/settings/log-level")
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
		level, _ := resp["level"].(string)
		levels, _ := resp["levels"].([]interface{})
		strs := make([]string, 0, len(levels))
		for _, l := range levels {
			if s, ok := l.(string); ok {
				strs = append(strs, s)
			}
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Current log level: %s\n", level)
		fmt.Fprintf(out, "Available levels:  %s\n", strings.Join(strs, ", "))
		return nil
	},
}

var settingsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set a setting value",
}

var settingsSetLogLevelCmd = &cobra.Command{
	Use:   "log-level <level>",
	Short: "Set the log level (fatal|error|warn|info|debug|trace)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		body := map[string]string{"level": args[0]}
		data, err := c.Put("/api/admin/settings/log-level", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd,"Log level set to '%s'.", args[0])
		return nil
	},
}

var settingsGetResponseCacheCmd = &cobra.Command{
	Use:   "response-cache",
	Short: "Show exact-match response cache status and stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/settings/response-cache")
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		var resp struct {
			Enabled    bool `json:"enabled"`
			TTLSeconds int  `json:"ttl_seconds"`
			Entries    int  `json:"entries"`
			TotalHits  int  `json:"total_hits"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		state := "disabled"
		if resp.Enabled {
			state = "enabled"
		}
		fmt.Fprintf(out, "Response cache: %s\n", state)
		fmt.Fprintf(out, "TTL:            %d seconds\n", resp.TTLSeconds)
		fmt.Fprintf(out, "Entries:        %d\n", resp.Entries)
		fmt.Fprintf(out, "Total hits:     %d\n", resp.TotalHits)
		return nil
	},
}

var settingsSetResponseCacheCmd = &cobra.Command{
	Use:   "response-cache <on|off>",
	Short: "Enable or disable the exact-match response cache",
	Args:  cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		body := map[string]interface{}{}
		switch strings.ToLower(args[0]) {
		case "on", "true", "enable", "enabled":
			body["enabled"] = true
		case "off", "false", "disable", "disabled":
			body["enabled"] = false
		default:
			return fmt.Errorf("expected 'on' or 'off', got %q", args[0])
		}
		if ttl, _ := cmd.Flags().GetInt("ttl"); ttl >= 0 {
			body["ttl_seconds"] = ttl
		}
		data, err := c.Put("/api/admin/settings/response-cache", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Response cache updated (%s).", args[0])
		return nil
	},
}

var settingsClearResponseCacheCmd = &cobra.Command{
	Use:   "clear-response-cache",
	Short: "Purge all cached responses",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Delete("/api/admin/settings/response-cache")
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Response cache cleared.")
		return nil
	},
}