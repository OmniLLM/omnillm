package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

var UsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show usage metrics for all providers",
	Long:  "Show backend metering usage metrics across providers, models, clients, and API shapes.",
	RunE:  runUsage,
}

var CheckUsageCmd = &cobra.Command{
	Use:        "check-usage",
	Short:      "Show usage metrics for all providers",
	Deprecated: "use 'usage' instead",
	RunE:       runUsage,
}

func init() {
	addUsageFlags(UsageCmd)
	addUsageFlags(CheckUsageCmd)
}

func addUsageFlags(cmd *cobra.Command) {
	cmd.Flags().String("provider-id", "", "Filter by provider instance ID")
	cmd.Flags().String("model-id", "", "Filter by model ID")
	cmd.Flags().String("client", "", "Filter by client")
	cmd.Flags().String("api-shape", "", "Filter by API shape (openai or anthropic)")
	cmd.Flags().String("since", "", "Filter from RFC3339 timestamp")
	cmd.Flags().String("until", "", "Filter until RFC3339 timestamp")
	cmd.Flags().String("breakdown", "provider", "Breakdown to show when not using JSON: provider/providers, model/models, client/clients, or none")
}

func runUsage(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	query := usageQuery(cmd)

	statsData, err := c.Get("/api/admin/metering/stats" + query)
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(statsData)
		return nil
	}

	var stats map[string]any
	if err := json.Unmarshal(statsData, &stats); err != nil {
		return fmt.Errorf("parse stats response: %w", err)
	}

	fmt.Println("Usage summary:")
	fmt.Println(strings.Repeat("─", 40))
	printUsageValue("Requests", stats["total_requests"])
	printUsageValue("Input tokens", stats["total_input_tokens"])
	printUsageValue("Output tokens", stats["total_output_tokens"])
	printUsageValue("Total tokens", stats["total_tokens"])
	printUsageValue("Average latency ms", stats["avg_latency_ms"])
	printUsageValue("Errors", stats["error_count"])

	breakdown, _ := cmd.Flags().GetString("breakdown")
	path, err := usageBreakdownPath(breakdown)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}

	breakdownData, err := c.Get(path + query)
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := json.Unmarshal(breakdownData, &resp); err != nil {
		return fmt.Errorf("parse breakdown response: %w", err)
	}
	items, _ := resp["items"].([]any)
	if len(items) == 0 {
		return nil
	}

	fmt.Println()
	fmt.Printf("Usage by %s:\n", breakdown)
	fmt.Println(strings.Repeat("─", 80))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		printUsageBreakdownRow(breakdown, row)
	}
	return nil
}

func usageQuery(cmd *cobra.Command) string {
	params := url.Values{}
	for _, name := range []string{"provider-id", "model-id", "client", "api-shape", "since", "until"} {
		value, _ := cmd.Flags().GetString(name)
		if value != "" {
			params.Set(strings.ReplaceAll(name, "-", "_"), value)
		}
	}
	if len(params) == 0 {
		return ""
	}
	return "?" + params.Encode()
}

func usageBreakdownPath(breakdown string) (string, error) {
	switch breakdown {
	case "provider", "providers":
		return "/api/admin/metering/by-provider", nil
	case "model", "models":
		return "/api/admin/metering/by-model", nil
	case "client", "clients":
		return "/api/admin/metering/by-client", nil
	case "", "none":
		return "", nil
	default:
		return "", fmt.Errorf("invalid breakdown %q: expected provider, model, client, or none", breakdown)
	}
}

func printUsageValue(label string, value any) {
	fmt.Printf("  %-20s %v\n", label+":", value)
}

func printUsageBreakdownRow(breakdown string, row map[string]any) {
	labelKey := map[string]string{
		"provider":  "provider_id",
		"providers": "provider_id",
		"model":     "model_id",
		"models":    "model_id",
		"client":    "client",
		"clients":   "client",
	}[breakdown]
	label, _ := row[labelKey].(string)
	if label == "" {
		label = "(unknown)"
	}
	fmt.Printf("  %-36s requests=%v tokens=%v avg_latency_ms=%v\n",
		padRight(label, 36), row["requests"], row["total_tokens"], row["avg_latency_ms"])
}
