// Package antigravity provides the Antigravity (Google Cloud Code internal API) provider.
package antigravity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"omnimodel/internal/cif"
	"omnimodel/internal/providers/shared"
	"omnimodel/internal/providers/types"
)

const defaultBaseURL = "https://daily-cloudcode-pa.googleapis.com"

// Models is the Antigravity model catalog.
var Models = []types.Model{
	{ID: "claude-opus-4-6-thinking", Name: "Claude Opus 4.6 (Thinking)", MaxTokens: 64000, Provider: "antigravity"},
	{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6 (Thinking)", MaxTokens: 64000, Provider: "antigravity"},
	{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", MaxTokens: 65535, Provider: "antigravity"},
	{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite", MaxTokens: 65535, Provider: "antigravity"},
	{ID: "gemini-3-flash", Name: "Gemini 3 Flash", MaxTokens: 65536, Provider: "antigravity"},
	{ID: "gemini-3-pro-high", Name: "Gemini 3 Pro (High)", MaxTokens: 65535, Provider: "antigravity"},
	{ID: "gemini-3-pro-low", Name: "Gemini 3 Pro (Low)", MaxTokens: 65535, Provider: "antigravity"},
	{ID: "gemini-3.1-flash-image", Name: "Gemini 3.1 Flash Image", MaxTokens: 65535, Provider: "antigravity"},
	{ID: "gemini-3.1-pro-high", Name: "Gemini 3.1 Pro (High)", MaxTokens: 65535, Provider: "antigravity"},
	{ID: "gemini-3.1-pro-low", Name: "Gemini 3.1 Pro (Low)", MaxTokens: 65535, Provider: "antigravity"},
	{ID: "gpt-oss-120b-medium", Name: "GPT-OSS 120B (Medium)", MaxTokens: 32768, Provider: "antigravity"},
}

// GetModels returns the available models for this Antigravity instance.
func GetModels(instanceID string) *types.ModelsResponse {
	result := make([]types.Model, len(Models))
	for i, m := range Models {
		result[i] = m
		result[i].Provider = instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

// RemapModel maps incoming model names to Antigravity-specific model IDs.
func RemapModel(model string) string {
	switch {
	case strings.HasPrefix(model, "claude-opus-4"):
		return "claude-opus-4-6-thinking"
	case strings.HasPrefix(model, "claude-sonnet-4"):
		return "claude-sonnet-4-6"
	case strings.HasPrefix(model, "claude-haiku-4"):
		return "gemini-3-flash"
	}
	return model
}

// Stream executes a streaming request to the Antigravity Cloud Code API.
func Stream(token, baseURL, projectID string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if token == "" {
		return nil, fmt.Errorf("antigravity: not authenticated (set access_token via admin UI)")
	}

	model := RemapModel(request.Model)
	contents := shared.CIFMessagesToGemini(request.Messages)

	geminiReq := map[string]interface{}{
		"sessionId": shared.RandomID(),
		"contents":  contents,
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
	if len(genConfig) > 0 {
		geminiReq["generationConfig"] = genConfig
	}

	payload := map[string]interface{}{
		"model":       model,
		"userAgent":   "antigravity/1.19.6",
		"requestType": "agent",
		"requestId":   shared.RandomID(),
		"request":     geminiReq,
	}
	if projectID != "" {
		payload["project"] = projectID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal antigravity request: %w", err)
	}

	base := baseURL
	if base == "" {
		base = defaultBaseURL
	}
	url := base + "/v1internal:streamGenerateContent?alt=sse"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/1.19.6")
	req.Header.Set("X-Goog-Api-Client", "google-cloud-sdk vscode_cloudshelleditor/0.1")
	req.Header.Set("Client-Metadata", `{"ideType":"IDE_UNSPECIFIED","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("antigravity request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("antigravity API failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go ParseAntigravitySSE(resp.Body, eventCh)
	return eventCh, nil
}

// Execute runs a non-streaming Antigravity request (via stream collection).
func Execute(token, baseURL, projectID string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	ch, err := Stream(token, baseURL, projectID, request)
	if err != nil {
		return nil, err
	}
	return shared.CollectStream(ch)
}
