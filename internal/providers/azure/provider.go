// Package azure provides Azure OpenAI provider implementation.
package azure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"omnimodel/internal/cif"
	"omnimodel/internal/database"
	"omnimodel/internal/providers/shared"
	"omnimodel/internal/providers/types"

	"github.com/rs/zerolog/log"
)

// Shared HTTP clients: one for normal requests with timeout, one for streaming.
var (
	azureHTTPClient   = &http.Client{Timeout: 120 * time.Second}
	azureStreamClient = &http.Client{} // no timeout for streaming
)

// ─── Auth ─────────────────────────────────────────────────────────────────────

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

// ─── URL helpers ──────────────────────────────────────────────────────────────

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

// ─── Headers ──────────────────────────────────────────────────────────────────

// Headers returns the HTTP headers for Azure OpenAI API requests.
func Headers(apiKey string) map[string]string {
	return map[string]string{
		"api-key":      apiKey,
		"Content-Type": "application/json",
	}
}

// ─── Models ───────────────────────────────────────────────────────────────────

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

// ─── Model detection ──────────────────────────────────────────────────────────

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

// ─── OpenAI payload builder ───────────────────────────────────────────────────

// BuildOpenAIPayload builds the Azure OpenAI chat completions request payload.
// The model field is omitted (deployment is in the URL for Azure).
func BuildOpenAIPayload(model string, messages []map[string]interface{}, request *cif.CanonicalRequest) map[string]interface{} {
	payload := map[string]interface{}{
		"messages": messages,
	}

	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		modelLower := strings.ToLower(model)
		if strings.Contains(modelLower, "gpt-5.3") ||
			strings.Contains(modelLower, "gpt-5.4") ||
			strings.Contains(modelLower, "gpt-6") {
			payload["max_completion_tokens"] = *request.MaxTokens
		} else {
			payload["max_tokens"] = *request.MaxTokens
		}
	}
	if len(request.Stop) > 0 {
		payload["stop"] = request.Stop
	}
	if request.UserID != nil {
		payload["user"] = *request.UserID
	}

	if len(request.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			t := map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":       tool.Name,
					"parameters": tool.ParametersSchema,
				},
			}
			if tool.Description != nil {
				t["function"].(map[string]interface{})["description"] = *tool.Description
			}
			tools = append(tools, t)
		}
		payload["tools"] = tools
	}

	if request.ToolChoice != nil {
		if toolChoice := shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice); toolChoice != nil {
			payload["tool_choice"] = toolChoice
		}
	}

	// Azure: model is in the URL (deployment), not the body
	// (model field intentionally omitted)

	return payload
}

// ─── Execute non-streaming ────────────────────────────────────────────────────

// ExecuteOpenAI executes a non-streaming OpenAI-compatible chat completion for Azure.
func ExecuteOpenAI(url string, headers map[string]string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	messages := shared.CIFMessagesToOpenAI(request.Messages)
	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		messages = append([]map[string]interface{}{{
			"role":    "system",
			"content": *request.SystemPrompt,
		}}, messages...)
	}

	payload := BuildOpenAIPayload(request.Model, messages, request)
	payload["stream"] = false

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := azureHTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return shared.OpenAIRespToCIF(openaiResp), nil
}

// ─── Execute streaming ────────────────────────────────────────────────────────

// StreamOpenAI executes a streaming OpenAI-compatible chat completion for Azure.
func StreamOpenAI(url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	messages := shared.CIFMessagesToOpenAI(request.Messages)
	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		messages = append([]map[string]interface{}{{
			"role":    "system",
			"content": *request.SystemPrompt,
		}}, messages...)
	}

	payload := BuildOpenAIPayload(request.Model, messages, request)
	payload["stream"] = true

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := azureStreamClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("streaming API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go shared.ParseOpenAISSE(resp.Body, eventCh)
	return eventCh, nil
}
