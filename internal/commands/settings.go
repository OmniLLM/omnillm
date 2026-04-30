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
	SettingsCmd.AddCommand(settingsGetCmd)

	settingsSetCmd.AddCommand(settingsSetLogLevelCmd)
	SettingsCmd.AddCommand(settingsSetCmd)
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
		fmt.Printf("Current log level: %s\n", level)
		fmt.Printf("Available levels:  %s\n", strings.Join(strs, ", "))
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
