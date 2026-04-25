package generic

import (
	"context"
	"fmt"
	"strings"

	antigravitypkg "omnillm/internal/providers/antigravity"
	alibabapkg "omnillm/internal/providers/alibaba"
	azurepkg "omnillm/internal/providers/azure"
	googlepkg "omnillm/internal/providers/google"
	kimipkg "omnillm/internal/providers/kimi"
	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
)

func (a *GenericAdapter) GetProvider() types.Provider { return a.provider }

func (a *GenericAdapter) RemapModel(model string) string {
	if a.provider.id == "antigravity" {
		return antigravitypkg.RemapModel(model)
	}
	if a.provider.id == "alibaba" {
		return alibabapkg.RemapModel(model)
	}
	if a.provider.id == "azure-openai" {
		return azurepkg.RemapModel(a.provider.config, strings.TrimSpace(model))
	}
	return strings.TrimSpace(model)
}

func (a *GenericAdapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.loadConfigFromDB()
	switch a.provider.id {
	case "alibaba":
		if !alibabapkg.IsChatCompletionsModel(a.RemapModel(request.Model)) {
			return nil, fmt.Errorf("alibaba model %s is realtime-only and is not supported by /v1/chat/completions", request.Model)
		}
		return a.executeOpenAI(ctx, a.alibabaURL(), a.provider.alibabaHeaders(false), request)
	case "azure-openai":
		return a.executeAzureResponses(ctx, request)
	case "google":
		return googlepkg.Execute(ctx, a.provider.token, a.provider.baseURL, request)
	case "kimi":
		return a.executeOpenAI(ctx, a.kimiURL(), a.kimiHeaders(false), request)
	case "antigravity":
		return a.collectStream(ctx, request)
	default:
		return nil, fmt.Errorf("provider %s not yet implemented", a.provider.id)
	}
}

func (a *GenericAdapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.loadConfigFromDB()
	switch a.provider.id {
	case "alibaba":
		if !alibabapkg.IsChatCompletionsModel(a.RemapModel(request.Model)) {
			return nil, fmt.Errorf("alibaba model %s is realtime-only and is not supported by /v1/chat/completions", request.Model)
		}
		return a.streamOpenAI(ctx, a.alibabaURL(), a.provider.alibabaHeaders(true), request)
	case "azure-openai":
		return a.streamAzureResponses(ctx, request)
	case "google":
		return googlepkg.Stream(ctx, a.provider.token, a.provider.baseURL, request)
	case "kimi":
		return a.streamOpenAI(ctx, a.kimiURL(), a.kimiHeaders(true), request)
	case "antigravity":
		projectID, _ := firstString(a.provider.config, "project_id", "project")
		return antigravitypkg.Stream(ctx, a.provider.token, a.provider.baseURL, projectID, request)
	default:
		return nil, fmt.Errorf("provider %s not yet implemented", a.provider.id)
	}
}

// ─── URL / header helpers ─────────────────────────────────────────────────────

func (a *GenericAdapter) alibabaURL() string {
	return alibabapkg.ChatURL(a.provider.baseURL)
}

func (a *GenericAdapter) alibabaHeaders(stream bool) map[string]string {
	token := a.provider.ensureFreshAlibabaToken()
	return alibabapkg.Headers(token, stream, a.provider.config)
}

func (a *GenericAdapter) kimiURL() string {
	return kimipkg.ChatURL(a.provider.baseURL)
}

func (a *GenericAdapter) kimiHeaders(stream bool) map[string]string {
	return kimipkg.Headers(a.provider.token, stream, a.provider.config)
}

func (a *GenericAdapter) azureResponsesURL() (string, error) {
	return azurepkg.ResponsesURL(a.provider.baseURL)
}

// ─── Azure Responses API ──────────────────────────────────────────────────────

func (a *GenericAdapter) executeAzureResponses(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	url, err := a.azureResponsesURL()
	if err != nil {
		return nil, err
	}
	model := a.RemapModel(request.Model)
	return azurepkg.ExecuteResponses(ctx, url, a.provider.token, request, model)
}

func (a *GenericAdapter) streamAzureResponses(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	url, err := a.azureResponsesURL()
	if err != nil {
		return nil, err
	}
	model := a.RemapModel(request.Model)
	return azurepkg.StreamResponses(ctx, url, a.provider.token, request, model)
}

func (a *GenericAdapter) buildOpenAIPayload(request *cif.CanonicalRequest) map[string]interface{} {
	model := a.RemapModel(request.Model)
	messages := shared.CIFMessagesToOpenAI(request.Messages)
	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		messages = append([]map[string]interface{}{{
			"role":    "system",
			"content": *request.SystemPrompt,
		}}, messages...)
	}

	if a.provider.id == "alibaba" {
		payload := map[string]interface{}{
			"model":    model,
			"messages": messages,
		}
		defTemp := 0.55
		defTopP := 1.0
		if request.Temperature != nil {
			payload["temperature"] = *request.Temperature
		} else {
			payload["temperature"] = defTemp
		}
		if request.TopP != nil {
			payload["top_p"] = *request.TopP
		} else {
			payload["top_p"] = defTopP
		}
		if request.MaxTokens != nil {
			payload["max_tokens"] = *request.MaxTokens
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
						"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
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
			if tc := shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice); tc != nil {
				payload["tool_choice"] = tc
			}
		}
		if alibabapkg.IsReasoningModel(model) && len(request.Tools) == 0 {
			payload["enable_thinking"] = true
		}
		return payload
	}

	// Default OpenAI-compatible payload
	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
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
					"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
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
	return payload
}

func (a *GenericAdapter) executeOpenAI(ctx context.Context, url string, headers map[string]string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = false
	return executeOpenAIWithPayload(ctx, url, headers, payload, request.Model)
}

func (a *GenericAdapter) streamOpenAI(ctx context.Context, url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.streamOpenAIGeneric(ctx, url, headers, request)
}

func (a *GenericAdapter) streamOpenAIGeneric(ctx context.Context, url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = true
	if a.provider.id == "alibaba" {
		payload["stream_options"] = map[string]interface{}{"include_usage": true}
	}
	return streamOpenAIWithPayload(ctx, url, headers, payload)
}

// collectStream runs ExecuteStream and assembles a CanonicalResponse.
func (a *GenericAdapter) collectStream(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	ch, err := a.ExecuteStream(ctx, request)
	if err != nil {
		return nil, err
	}
	return shared.CollectStream(ch)
}

// ─── Google payload builder (kept for white-box test access) ──────────────────

func (a *GenericAdapter) buildGooglePayload(request *cif.CanonicalRequest) map[string]interface{} {
	return googlepkg.BuildPayload(a.RemapModel(request.Model), request)
}

func (a *GenericAdapter) googleURL(model string) string {
	return googlepkg.StreamURL(a.provider.baseURL, model)
}

func (a *GenericAdapter) executeGoogle(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return googlepkg.Execute(ctx, a.provider.token, a.provider.baseURL, request)
}

func (a *GenericAdapter) streamGoogle(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return googlepkg.Stream(ctx, a.provider.token, a.provider.baseURL, request)
}

// ─── Antigravity stream (kept for white-box test access) ─────────────────────

func (a *GenericAdapter) streamAntigravity(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	projectID, _ := firstString(a.provider.config, "project_id", "project")
	return antigravitypkg.Stream(ctx, a.provider.token, a.provider.baseURL, projectID, request)
}
