package azure

import (
	"fmt"
	"strings"

	"omnillm/internal/database"
	"omnillm/internal/providers/types"

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
		cfg["endpoint"] = options.Endpoint
		resolvedEndpoint = strings.TrimRight(options.Endpoint, "/")
		configStore := database.NewProviderConfigStore()
		if saveErr := configStore.Save(instanceID, cfg); saveErr != nil {
			log.Warn().Err(saveErr).Str("provider", instanceID).Msg("Azure: failed to save endpoint config")
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
	return endpoint + "/openai/v1/responses", nil
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

// GetModels returns the available models for this Azure OpenAI instance.
// If the config contains a "deployments" list those are used; otherwise the
// built-in catalog is returned.
func GetModels(instanceID string, config map[string]interface{}) *types.ModelsResponse {
	if deployments := stringSliceFromConfig(config, "deployments"); len(deployments) > 0 {
		models := make([]types.Model, 0, len(deployments))
		for _, d := range deployments {
			models = append(models, types.Model{
				ID:        d,
				Name:      d,
				MaxTokens: 128000,
				Provider:  instanceID,
			})
		}
		return &types.ModelsResponse{Data: models, Object: "list"}
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

// IsResponsesAPIModel returns true if the model should use the Responses API
// rather than the chat completions endpoint.
func IsResponsesAPIModel(model string) bool {
	modelLower := strings.ToLower(model)
	patterns := []string{
		"gpt-5.1-codex",
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"gpt-5-codex",
		"gpt-5.4",
	}
	for _, p := range patterns {
		if strings.Contains(modelLower, p) {
			return true
		}
	}
	return false
}
