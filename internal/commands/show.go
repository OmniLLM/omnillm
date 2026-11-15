package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var providerShowCmd = &cobra.Command{
	Use:   "show <reference>",
	Short: "Show details of a provider",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(cmd)
		reference, err := resolveProviderID(cmd, client, args)
		if err != nil {
			return err
		}
		data, err := client.Get("/api/admin/providers")
		if err != nil {
			return err
		}
		providers, err := parseProviders(data)
		if err != nil {
			return err
		}
		provider, err := findProviderForShow(providers, reference)
		if err != nil {
			return err
		}
		details := safeProviderDetails(provider)
		if client.IsJSON() {
			return writeShowJSON(cmd, details)
		}
		return renderShowDetails(cmd, details, []showField{
			{key: "id", label: "ID"},
			{key: "type", label: "Type"},
			{key: "name", label: "Name"},
			{key: "alias", label: "Alias"},
			{key: "authStatus", label: "Auth status"},
			{key: "isActive", label: "Active"},
			{key: "enabledModelCount", label: "Enabled models"},
			{key: "totalModelCount", label: "Total models"},
		})
	},
}

var modelShowCmd = &cobra.Command{
	Use:               "show <reference> <model-id>",
	Short:             "Show details of a provider model",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: modelProviderCompletionFunc,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(cmd)
		path := "/api/admin/providers/" + url.PathEscape(args[0]) + "/models"
		data, err := client.Get(path)
		if err != nil {
			var responseError *serverResponseError
			if errors.As(err, &responseError) && responseError.code == "ambiguous_provider_reference" && responseError.message != "" {
				return fmt.Errorf("%w: %s", err, responseError.message)
			}
			return err
		}
		var response struct {
			Models []map[string]any `json:"models"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return fmt.Errorf("parse provider models: %w", err)
		}
		for _, model := range response.Models {
			if id, ok := model["id"].(string); ok && id != "" && id == args[1] {
				if client.IsJSON() {
					return writeShowJSON(cmd, model)
				}
				return renderShowDetails(cmd, model, []showField{
					{key: "id", label: "ID"},
					{key: "name", label: "Name"},
					{key: "enabled", label: "Enabled"},
					{key: "description", label: "Description"},
					{key: "max_tokens", label: "Max tokens"},
					{key: "output_tokens", label: "Output tokens"},
					{key: "capabilities", label: "Capabilities"},
				})
			}
		}
		return fmt.Errorf("model %q not found for provider %q", args[1], args[0])
	},
}

// findProviderForShow follows the server's ID, alias, and name precedence.
func findProviderForShow(providers []map[string]any, reference string) (map[string]any, error) {
	reference = strings.TrimSpace(reference)
	for _, provider := range providers {
		id, _ := provider["id"].(string)
		if id != "" && id == reference {
			return provider, nil
		}
	}
	key := strings.ToLower(reference)
	if key != "" {
		for _, field := range []string{"alias", "name"} {
			matches := []map[string]any{}
			ids := []string{}
			for _, provider := range providers {
				value, _ := provider[field].(string)
				if field == "alias" && value == "" {
					value, _ = provider["subtitle"].(string)
				}
				if strings.ToLower(strings.TrimSpace(value)) != key {
					continue
				}
				id, _ := provider["id"].(string)
				if id == "" {
					continue
				}
				matches = append(matches, provider)
				ids = append(ids, id)
			}
			switch len(matches) {
			case 0:
				continue
			case 1:
				return matches[0], nil
			default:
				slices.Sort(ids)
				return nil, fmt.Errorf("provider reference %q is ambiguous; matching instance IDs: %s", reference, strings.Join(ids, ", "))
			}
		}
	}
	return nil, fmt.Errorf("provider reference %q not found", reference)
}

// safeProviderDetails excludes arbitrary provider configuration and unknown fields.
func safeProviderDetails(provider map[string]any) map[string]any {
	details := map[string]any{}
	for _, key := range []string{"id", "type", "name", "alias", "subtitle", "authStatus"} {
		if value, ok := provider[key].(string); ok {
			details[key] = value
		}
	}
	if value, ok := provider["isActive"].(bool); ok {
		details["isActive"] = value
	}
	for _, key := range []string{"totalModelCount", "enabledModelCount"} {
		if value, ok := provider[key].(float64); ok {
			details[key] = value
		}
	}
	if alias, _ := details["alias"].(string); alias == "" {
		if subtitle, ok := details["subtitle"].(string); ok {
			details["alias"] = subtitle
		}
	}
	return details
}

type showField struct {
	key   string
	label string
}

func writeShowJSON(cmd *cobra.Command, details map[string]any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(details); err != nil {
		return fmt.Errorf("write details: %w", err)
	}
	return nil
}

func renderShowDetails(cmd *cobra.Command, details map[string]any, fields []showField) error {
	table := NewTable("Field", "Value")
	for _, field := range fields {
		value, ok := details[field.key]
		if !ok || value == nil {
			continue
		}
		formatted := fmt.Sprint(value)
		if field.key == "capabilities" {
			encoded, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("format capabilities: %w", err)
			}
			formatted = string(encoded)
		}
		table.AddRow(field.label, formatted)
	}
	return table.Render(cmd.OutOrStdout())
}
