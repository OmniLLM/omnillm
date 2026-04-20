package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
)

// Shared HTTP clients: one for normal requests with timeout, one for streaming.
var (
	azureHTTPClient = &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	azureStreamClient = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
)

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

// ExecuteOpenAI executes a non-streaming OpenAI-compatible chat completion for Azure.
func ExecuteOpenAI(ctx context.Context, url string, headers map[string]string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
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

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := azureHTTPClient.Do(req)
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

// StreamOpenAI executes a streaming OpenAI-compatible chat completion for Azure.
func StreamOpenAI(ctx context.Context, url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
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

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := azureStreamClient.Do(req)
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
