// Package azure — Azure Responses API implementation.
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
)

// Shared HTTP client with default timeout for Responses API requests.
var responsesHTTPClient = &http.Client{
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

// ─── Tool call ID normalization ───────────────────────────────────────────────

// ToolCallID ensures a tool call ID starts with "fc" as required by Azure.
func ToolCallID(id string) string {
	if strings.HasPrefix(id, "fc") {
		return id
	}
	stripped := strings.TrimPrefix(id, "call_")
	return "fc_" + stripped
}

// ─── Payload builder ──────────────────────────────────────────────────────────

// BuildResponsesPayload converts a CIF request to the Azure Responses API format.
func BuildResponsesPayload(request *cif.CanonicalRequest, model string) map[string]interface{} {
	input := CIFMessagesToResponsesInput(request.Messages)

	maxOutputTokens := 4000
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		maxOutputTokens = *request.MaxTokens
		if maxOutputTokens < 16 {
			maxOutputTokens = 16
		}
	}

	payload := map[string]interface{}{
		"model":             model,
		"input":             input,
		"max_output_tokens": maxOutputTokens,
		"generate":          true,
		"store":             false,
	}

	if request.SystemPrompt != nil && *request.SystemPrompt != "" {
		payload["instructions"] = *request.SystemPrompt
	}

	// gpt-5.4-pro and gpt-5.1-codex-max do not support temperature
	modelLower := strings.ToLower(model)
	supportsTemperature := !strings.Contains(modelLower, "gpt-5.4-pro") &&
		!strings.Contains(modelLower, "gpt-5.1-codex-max")
	if supportsTemperature {
		if request.Temperature != nil {
			payload["temperature"] = *request.Temperature
		} else {
			payload["temperature"] = 0.1
		}
	}

	if len(request.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			t := map[string]interface{}{
				"type":       "function",
				"name":       tool.Name,
				"parameters": tool.ParametersSchema,
			}
			if tool.Description != nil {
				t["description"] = *tool.Description
			}
			tools = append(tools, t)
		}
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	return payload
}

// CIFMessagesToResponsesInput converts CIF messages to the Azure Responses API input array.
func CIFMessagesToResponsesInput(messages []cif.CIFMessage) []map[string]interface{} {
	var input []map[string]interface{}

	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			input = append(input, map[string]interface{}{
				"type": "message",
				"role": "system",
				"content": []map[string]interface{}{
					{"type": "input_text", "text": m.Content},
				},
			})
		case cif.CIFUserMessage:
			var textBlocks []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBlocks = append(textBlocks, map[string]interface{}{
						"type": "input_text",
						"text": p.Text,
					})
				case cif.CIFToolResultPart:
					if len(textBlocks) > 0 {
						input = append(input, map[string]interface{}{
							"type":    "message",
							"role":    "user",
							"content": textBlocks,
						})
						textBlocks = nil
					}
					callID := ToolCallID(p.ToolCallID)
					input = append(input, map[string]interface{}{
						"type":    "function_call_output",
						"call_id": callID,
						"output":  p.Content,
					})
				}
			}
			if len(textBlocks) > 0 {
				input = append(input, map[string]interface{}{
					"type":    "message",
					"role":    "user",
					"content": textBlocks,
				})
			}
		case cif.CIFAssistantMessage:
			var textBlocks []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBlocks = append(textBlocks, map[string]interface{}{
						"type": "output_text",
						"text": p.Text,
					})
				case cif.CIFToolCallPart:
					if len(textBlocks) > 0 {
						input = append(input, map[string]interface{}{
							"type":    "message",
							"role":    "assistant",
							"content": textBlocks,
						})
						textBlocks = nil
					}
					callID := ToolCallID(p.ToolCallID)
					argsBytes, _ := json.Marshal(p.ToolArguments)
					input = append(input, map[string]interface{}{
						"type":      "function_call",
						"id":        callID,
						"call_id":   callID,
						"name":      p.ToolName,
						"arguments": string(argsBytes),
					})
				}
			}
			if len(textBlocks) > 0 {
				input = append(input, map[string]interface{}{
					"type":    "message",
					"role":    "assistant",
					"content": textBlocks,
				})
			}
		}
	}

	return input
}

// ─── Response conversion ──────────────────────────────────────────────────────

// ResponsesRespToCIF converts an Azure Responses API response to CIF format.
func ResponsesRespToCIF(resp map[string]interface{}, originalModel string) *cif.CanonicalResponse {
	id, _ := resp["id"].(string)
	if id == "" {
		id = fmt.Sprintf("resp_%d", time.Now().UnixMilli())
	}

	var content []cif.CIFContentPart
	stopReason := cif.StopReasonEndTurn

	output, _ := resp["output"].([]interface{})
	for _, item := range output {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		switch itemType {
		case "message":
			contentItems, _ := itemMap["content"].([]interface{})
			for _, block := range contentItems {
				blockMap, ok := block.(map[string]interface{})
				if !ok {
					continue
				}
				if blockType, _ := blockMap["type"].(string); blockType == "output_text" {
					text, _ := blockMap["text"].(string)
					content = append(content, cif.CIFTextPart{Type: "text", Text: text})
				}
			}
		case "function_call":
			callID, _ := itemMap["id"].(string)
			if callID == "" {
				callID, _ = itemMap["call_id"].(string)
			}
			name, _ := itemMap["name"].(string)
			argsStr, _ := itemMap["arguments"].(string)
			var args map[string]interface{}
			json.Unmarshal([]byte(argsStr), &args) //nolint:errcheck
			if args == nil {
				args = map[string]interface{}{}
			}
			content = append(content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    callID,
				ToolName:      name,
				ToolArguments: args,
			})
			stopReason = cif.StopReasonToolUse
		}
	}

	if status, _ := resp["status"].(string); status == "incomplete" {
		stopReason = cif.StopReasonMaxTokens
	}

	var usage *cif.CIFUsage
	if usageMap, ok := resp["usage"].(map[string]interface{}); ok {
		inputTokens, _ := usageMap["input_tokens"].(float64)
		outputTokens, _ := usageMap["output_tokens"].(float64)
		usage = &cif.CIFUsage{InputTokens: int(inputTokens), OutputTokens: int(outputTokens)}
	}

	return &cif.CanonicalResponse{
		ID:         id,
		Model:      originalModel,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}
}

// ─── Execute (non-streaming) ──────────────────────────────────────────────────

// ExecuteResponses calls the Azure Responses API (non-streaming) and returns a CIF response.
func ExecuteResponses(ctx context.Context, responsesURL, apiKey string, request *cif.CanonicalRequest, model string) (*cif.CanonicalResponse, error) {
	payload := BuildResponsesPayload(request, model)
	payload["stream"] = false

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", responsesURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range Headers(apiKey) {
		req.Header.Set(k, v)
	}

	client := responsesHTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	var respMap map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respMap); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return ResponsesRespToCIF(respMap, request.Model), nil
}

// ─── Stream (wraps non-streaming via event channel) ──────────────────────────

// StreamResponses calls ExecuteResponses and emits the result as a CIF stream.
// The Azure Responses API SSE format is complex; non-streaming is simpler and sufficient.
func StreamResponses(ctx context.Context, responsesURL, apiKey string, request *cif.CanonicalRequest, model string) (<-chan cif.CIFStreamEvent, error) {
	cifResp, err := ExecuteResponses(ctx, responsesURL, apiKey, request, model)
	if err != nil {
		return nil, err
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go func() {
		defer close(eventCh)
		eventCh <- cif.CIFStreamStart{Type: "stream_start", ID: cifResp.ID, Model: cifResp.Model}
		for i, part := range cifResp.Content {
			switch p := part.(type) {
			case cif.CIFTextPart:
				eventCh <- cif.CIFContentDelta{
					Type:         "content_delta",
					Index:        i,
					ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
					Delta:        cif.TextDelta{Type: "text_delta", Text: p.Text},
				}
			case cif.CIFToolCallPart:
				argsBytes, _ := json.Marshal(p.ToolArguments)
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: i,
					ContentBlock: cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    p.ToolCallID,
						ToolName:      p.ToolName,
						ToolArguments: map[string]interface{}{},
					},
					Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: string(argsBytes)},
				}
			}
		}
		eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cifResp.StopReason, Usage: cifResp.Usage}
	}()

	return eventCh, nil
}
