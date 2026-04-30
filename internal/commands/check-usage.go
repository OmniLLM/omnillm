package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var CheckUsageCmd = &cobra.Command{
	Use:   "check-usage",
	Short: "Check GitHub Copilot usage and quota",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)

		// Get usage from the backend metering endpoint
		data, err := c.Get("/api/metering/usage")
		if err != nil {
			return fmt.Errorf("failed to get usage: %w", err)
		}

		var usage map[string]interface{}
		if err := json.Unmarshal(data, &usage); err != nil {
			fmt.Println(string(data))
			return nil
		}

		fmt.Println("Copilot Usage:")
		fmt.Println(strings.Repeat("─", 30))

		for key, val := range usage {
			fmt.Printf("  %-20s %v\n", key+":", val)
		}

		return nil
	},
}
