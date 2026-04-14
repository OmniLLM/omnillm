// Package google provides Google Gemini provider implementation.
package google

import (
	"bytes"
	"context"
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

const defaultBaseURL = "https://generativelanguage.googleapis.com"

// Shared HTTP clients: one for normal requests with timeout, one for streaming.
var (
	googleHTTPClient   = &http.Client{Timeout: 120 * time.Second}
	googleStreamClient = &http.Client{}
)

// ─── Auth ─────────────────────────────────────────────────────────────────────

// SetupAuth handles API-key authentication for Google Gemini.
// Returns the token, base URL, and display name; or an error.
func SetupAuth(instanceID string, options *types.AuthOptions) (token, baseURL, name string, err error) {
	if options.APIKey == "" {
		return "", "", "", fmt.Errorf("google: API key is required")
	}

	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{"access_token": options.APIKey}
	if err := tokenStore.Save(instanceID, "google", tokenData); err != nil {
		return "", "", "", fmt.Errorf("failed to save google token: %w", err)
	}

	suffix := shared.ShortTokenSuffix(options.APIKey)
	displayName := "Google Gemini (" + suffix + ")"

	log.Info().Str("provider", instanceID).Msg("Google Gemini authenticated via API key")
	return options.APIKey, defaultBaseURL, displayName, nil
}

// ─── Headers ──────────────────────────────────────────────────────────────────

// Headers returns HTTP headers for Google Gemini API requests.
func Headers(apiKey string) map[string]string {
	return map[string]string{
		"x-goog-api-key": apiKey,
		"Content-Type":   "application/json",
	}
}

// ─── Models ───────────────────────────────────────────────────────────────────

// FetchModels calls the Google Gemini REST API (GET /v1beta/models) and returns
// all models that support generateContent (required for chat).
func FetchModels(instanceID, token, baseURL string) (*types.ModelsResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("google: not authenticated")
	}

	base := baseURL
	if base == "" {
		base = defaultBaseURL
	}

	url := base + "/v1beta/models"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("google: failed to build list-models request: %w", err)
	}
	req.Header.Set("x-goog-api-key", token)

	client := googleHTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google: list-models request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google: list-models failed with status %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			Description                string   `json:"description"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("google: failed to parse list-models response: %w", err)
	}

	var models []types.Model
	for _, m := range raw.Models {
		supportsChat := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsChat = true
				break
			}
		}
		if !supportsChat {
			continue
		}

		modelID := strings.TrimPrefix(m.Name, "models/")
		name := m.DisplayName
		if name == "" {
			name = modelID
		}
		maxTokens := m.OutputTokenLimit
		if maxTokens == 0 {
			maxTokens = 8192
		}

		models = append(models, types.Model{
			ID:          modelID,
			Name:        name,
			Description: m.Description,
			MaxTokens:   maxTokens,
			Provider:    instanceID,
		})
	}

	log.Info().Str("provider", instanceID).Int("count", len(models)).Msg("Fetched Google models from API")
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}

// ─── URL helper ───────────────────────────────────────────────────────────────

// StreamURL builds the SSE URL for a given model.
func StreamURL(baseURL, model string) string {
	base := baseURL
	if base == "" {
		base = defaultBaseURL
	}
	return base + "/v1beta/models/" + model + ":streamGenerateContent?alt=sse"
}

// ─── Payload builder ──────────────────────────────────────────────────────────

// BuildPayload builds the request body for the Gemini API.
func BuildPayload(model string, request *cif.CanonicalRequest) map[string]interface{} {
	contents := shared.CIFMessagesToGemini(request.Messages)

	payload := map[string]interface{}{
		"model":    model,
		"contents": contents,
	}

	if request.SystemPrompt != nil && *request.SystemPrompt != "" {
		payload["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": *request.SystemPrompt},
			},
		}
	}

	if len(request.Tools) > 0 {
		decls := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			decl := map[string]interface{}{
				"name":       tool.Name,
				"parameters": shared.SanitizeGeminiSchema(tool.ParametersSchema),
			}
			if tool.Description != nil {
				decl["description"] = *tool.Description
			}
			decls = append(decls, decl)
		}
		payload["tools"] = []map[string]interface{}{
			{"functionDeclarations": decls},
		}
	}

	genConfig := map[string]interface{}{}
	if request.MaxTokens != nil {
		genConfig["maxOutputTokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		genConfig["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		genConfig["topP"] = *request.TopP
	}
	if request.Stop != nil {
		genConfig["stopSequences"] = request.Stop
	}
	if len(genConfig) > 0 {
		payload["generationConfig"] = genConfig
	}

	return payload
}

// ─── Stream ───────────────────────────────────────────────────────────────────

// Stream executes a streaming request to the Gemini API and returns a CIF event channel.
func Stream(token, baseURL string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if token == "" {
		return nil, fmt.Errorf("google: not authenticated (set API key via admin UI)")
	}

	model := request.Model
	contents := shared.CIFMessagesToGemini(request.Messages)

	geminiReq := map[string]interface{}{
		"contents": contents,
	}

	if request.SystemPrompt != nil && *request.SystemPrompt != "" {
		geminiReq["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": *request.SystemPrompt},
			},
		}
	}

	if len(request.Tools) > 0 {
		decls := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			decl := map[string]interface{}{
				"name":       tool.Name,
				"parameters": shared.SanitizeGeminiSchema(tool.ParametersSchema),
			}
			if tool.Description != nil {
				decl["description"] = *tool.Description
			}
			decls = append(decls, decl)
		}
		geminiReq["tools"] = []map[string]interface{}{
			{"functionDeclarations": decls},
		}
	}

	genConfig := map[string]interface{}{}
	if request.MaxTokens != nil {
		genConfig["maxOutputTokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		genConfig["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		genConfig["topP"] = *request.TopP
	}
	if request.Stop != nil {
		genConfig["stopSequences"] = request.Stop
	}
	if len(genConfig) > 0 {
		geminiReq["generationConfig"] = genConfig
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal google request: %w", err)
	}

	url := StreamURL(baseURL, model)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range Headers(token) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := googleStreamClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("google API failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go ParseGeminiSSE(resp.Body, eventCh)
	return eventCh, nil
}

// Execute runs a non-streaming request (implemented via streaming + collection).
func Execute(token, baseURL string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	ch, err := Stream(token, baseURL, request)
	if err != nil {
		return nil, err
	}
	return shared.CollectStream(ch)
}
