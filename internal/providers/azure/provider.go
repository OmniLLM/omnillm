package azure

import (
	"encoding/json"
	"fmt"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"strings"

	"github.com/rs/zerolog/log"
)

// SetupAuth handles API-key authentication for Azure OpenAI.
// Returns the token, endpoint, and config; or an error.
func SetupAuth(instanceID string, options *types.AuthOptions) (token, endpoint string, config map[string]interface{}, err error) {
	if options.APIKey == "" {
		return "", "", nil, fmt.Errorf("azure-openai: API key is required")
	}

	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{"access_token": options.APIKey}
	if err := tokenStore.Save(instanceID, "azure-openai", tokenData); err != nil {
		return "", "", nil, fmt.Errorf("failed to save azure token: %w", err)
	}

	cfg := map[string]interface{}{}
	resolvedEndpoint := ""
	if options.Endpoint != "" {
		trimmed := strings.TrimRight(strings.TrimSpace(options.Endpoint), "/")
		cfg["endpoint"] = trimmed
		resolvedEndpoint = trimmed
	}

	// Persist deployments if provided (JSON-encoded string) - accept strings or structured mappings
	if options.Deployments != "" {
		var raw []interface{}
		if err := json.Unmarshal([]byte(options.Deployments), &raw); err == nil {
			out := make([]interface{}, 0, len(raw))
			for _, item := range raw {
				switch v := item.(type) {
				case string:
					trim := strings.TrimSpace(v)
					if trim == "" {
						continue
					}
					out = append(out, trim)
				case map[string]interface{}:
					m := map[string]interface{}{}
					if mv, ok := v["model"].(string); ok {
						m["model"] = strings.TrimSpace(mv)
					}
					if dv, ok := v["deployment"].(string); ok {
						m["deployment"] = strings.TrimSpace(dv)
					}
					if len(m) > 0 {
						out = append(out, m)
					}
				}
			}
			if len(out) > 0 {
				cfg["deployments"] = out
			}
		}
	}

	// Persist API version if provided (store under api_version)
	if options.APIVersion != "" {
		cfg["api_version"] = options.APIVersion
	}

	// Save config if it contains any keys
	if len(cfg) > 0 {
		configStore := database.NewProviderConfigStore()
		if saveErr := configStore.Save(instanceID, cfg); saveErr != nil {
			log.Warn().Err(saveErr).Str("provider", instanceID).Msg("Azure: failed to save config")
		}
	}

	log.Info().Str("provider", instanceID).Msg("Azure OpenAI authenticated via API key")
	return options.APIKey, resolvedEndpoint, cfg, nil
}

// ChatURL builds the Azure OpenAI chat completions URL for a deployment.
func ChatURL(endpoint, deployment, apiVersion string) (string, error) {
	if endpoint == "" {
		return "", fmt.Errorf("azure-openai endpoint not configured; set it via the admin UI")
	}
	if apiVersion == "" {
		apiVersion = "2024-08-01-preview"
	}
	return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		endpoint, deployment, apiVersion), nil
}

// ResponsesURL builds the Azure OpenAI Responses API URL.
func ResponsesURL(endpoint string) (string, error) {
	if endpoint == "" {
		return "", fmt.Errorf("azure-openai endpoint not configured; set it via the admin UI")
	}
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(trimmed, "/openai/v1") {
		return trimmed + "/responses", nil
	}
	return trimmed + "/openai/v1/responses", nil
}

// Headers returns the HTTP headers for Azure OpenAI API requests.
func Headers(apiKey string) map[string]string {
	return map[string]string{
		"api-key":      apiKey,
		"Content-Type": "application/json",
	}
}

// DefaultModels is the built-in model catalog for Azure OpenAI.
var DefaultModels = []types.Model{
	{ID: "gpt-4o", Name: "GPT-4o", MaxTokens: 128000, Provider: "azure-openai"},
	{ID: "gpt-4o-mini", Name: "GPT-4o Mini", MaxTokens: 128000, Provider: "azure-openai"},
}

type DeploymentMapping struct {
	Model      string `json:"model"`
	Deployment string `json:"deployment"`
}

// DeploymentMappings returns Azure deployment mappings from config.
// It supports both legacy string deployments and structured model/deployment mappings.
func DeploymentMappings(config map[string]interface{}) []DeploymentMapping {
	if config == nil {
		return nil
	}
	raw, exists := config["deployments"]
	if !exists {
		return nil
	}

	switch value := raw.(type) {
	case []string:
		result := make([]DeploymentMapping, 0, len(value))
		for _, deployment := range value {
			deployment = strings.TrimSpace(deployment)
			if deployment == "" {
				continue
			}
			result = append(result, DeploymentMapping{Model: deployment, Deployment: deployment})
		}
		return result
	case []interface{}:
		result := make([]DeploymentMapping, 0, len(value))
		for _, item := range value {
			switch typed := item.(type) {
			case string:
				deployment := strings.TrimSpace(typed)
				if deployment == "" {
					continue
				}
				result = append(result, DeploymentMapping{Model: deployment, Deployment: deployment})
			case map[string]interface{}:
				model, _ := typed["model"].(string)
				deployment, _ := typed["deployment"].(string)
				model = strings.TrimSpace(model)
				deployment = strings.TrimSpace(deployment)
				if model == "" && deployment == "" {
					continue
				}
				if model == "" {
					model = deployment
				}
				if deployment == "" {
					deployment = model
				}
				result = append(result, DeploymentMapping{Model: model, Deployment: deployment})
			}
		}
		return result
	default:
		return nil
	}
}

// normalizeAzureConfig applies canonical storage keys for Azure provider configs.
func normalizeAzureConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return map[string]interface{}{}
	}
	norm := make(map[string]interface{})
	if ep, ok := config["endpoint"].(string); ok && ep != "" {
		norm["endpoint"] = strings.TrimRight(strings.TrimSpace(ep), "/")
	}
	if av, ok := config["api_version"].(string); ok && av != "" {
		norm["api_version"] = av
	} else if av, ok := config["apiVersion"].(string); ok && av != "" {
		norm["api_version"] = av
	}
	if raw, ok := config["deployments"]; ok {
		// preserve as-is but normalize inner maps
		switch v := raw.(type) {
		case []string:
			if len(v) > 0 {
				out := make([]interface{}, 0, len(v))
				for _, s := range v {
					s = strings.TrimSpace(s)
					if s != "" {
						out = append(out, s)
					}
				}
				if len(out) > 0 {
					norm["deployments"] = out
				}
			}
		case []interface{}:
			if len(v) > 0 {
				out := make([]interface{}, 0, len(v))
				for _, item := range v {
					switch it := item.(type) {
					case string:
						s := strings.TrimSpace(it)
						if s != "" {
							out = append(out, s)
						}
					case map[string]interface{}:
						m := map[string]interface{}{}
						if mv, ok := it["model"].(string); ok && mv != "" {
							m["model"] = strings.TrimSpace(mv)
						}
						if dv, ok := it["deployment"].(string); ok && dv != "" {
							m["deployment"] = strings.TrimSpace(dv)
						}
						if len(m) > 0 {
							out = append(out, m)
						}
					}
				}
				if len(out) > 0 {
					norm["deployments"] = out
				}
			}
		}
	}
	return norm
}

// backfillDeploymentsFromModelState will populate deployments in config when absent
// by reading model state records for this instance and creating model=deployment mappings
// (only when helpful). When it writes a repaired config it persists it and returns true.
func backfillDeploymentsFromModelState(instanceID string, config map[string]interface{}) (map[string]interface{}, bool, error) {
	if config == nil {
		config = map[string]interface{}{}
	}
	if len(DeploymentMappings(config)) > 0 {
		return config, false, nil
	}
	stateStore := database.NewModelStateStore()
	states, err := stateStore.GetAllByInstance(instanceID)
	if err != nil {
		return config, false, err
	}
	if len(states) == 0 {
		return config, false, nil
	}
	// Build structured mappings model=deployment when model IDs look usable
	out := make([]interface{}, 0, len(states))
	seen := make(map[string]struct{})
	for _, s := range states {
		id := strings.TrimSpace(s.ModelID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		m := map[string]interface{}{"model": id, "deployment": id}
		out = append(out, m)
	}
	if len(out) == 0 {
		return config, false, nil
	}
	config["deployments"] = out
	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(instanceID, config); err != nil {
		return config, false, err
	}
	return config, true, nil
}

// RemapModel maps an app-facing Azure model name to the configured Azure deployment name.
// If no mapping exists, it returns the original model unchanged.
func RemapModel(config map[string]interface{}, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return model
	}
	for _, mapping := range DeploymentMappings(config) {
		if strings.EqualFold(strings.TrimSpace(mapping.Model), model) {
			return mapping.Deployment
		}
	}
	return model
}

// ModelIDs returns the app-facing Azure model IDs from config.
func ModelIDs(config map[string]interface{}) []string {
	mappings := DeploymentMappings(config)
	result := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.Model != "" {
			result = append(result, mapping.Model)
		}
	}
	return result
}

// GetModels returns the available models for this Azure OpenAI instance.
// If the config contains deployment mappings those are used; otherwise the
// built-in catalog is returned. This will attempt to backfill deployments
// from model state when no deployments are configured.
func GetModels(instanceID string, config map[string]interface{}) *types.ModelsResponse {
	if mappings := DeploymentMappings(config); len(mappings) > 0 {
		models := make([]types.Model, 0, len(mappings))
		for _, mapping := range mappings {
			models = append(models, types.Model{
				ID:        mapping.Model,
				Name:      mapping.Model,
				MaxTokens: 128000,
				Provider:  instanceID,
			})
		}
		return &types.ModelsResponse{Data: models, Object: "list"}
	}

	// Try backfilling missing deployments from model state
	if cfg, ok := database.NewProviderConfigStore().Get(instanceID); ok == nil && cfg != nil {
		var loaded map[string]interface{}
		_ = json.Unmarshal([]byte(cfg.ConfigData), &loaded)
		if updated, wrote, err := backfillDeploymentsFromModelState(instanceID, loaded); err == nil && wrote {
			// use updated deployments
			if mappings := DeploymentMappings(updated); len(mappings) > 0 {
				models := make([]types.Model, 0, len(mappings))
				for _, mapping := range mappings {
					models = append(models, types.Model{
						ID:        mapping.Model,
						Name:      mapping.Model,
						MaxTokens: 128000,
						Provider:  instanceID,
					})
				}
				return &types.ModelsResponse{Data: models, Object: "list"}
			}
		}
	}

	result := make([]types.Model, len(DefaultModels))
	for i, m := range DefaultModels {
		result[i] = m
		result[i].Provider = instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

func stringSliceFromConfig(config map[string]interface{}, key string) []string {
	if config == nil {
		return nil
	}
	raw, exists := config[key]
	if !exists {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return value
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

// IsResponsesAPIModel returns true for all Azure OpenAI models.
// Azure OpenAI uses the Responses API exclusively


func IsResponsesAPIModel(_ string) bool {
	return true
}
