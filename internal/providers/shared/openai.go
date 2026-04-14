// Package shared provides utilities shared across multiple provider implementations.
package shared

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	"omnimodel/internal/cif"

	"github.com/rs/zerolog/log"
)

// ─── CIF → OpenAI message conversion ─────────────────────────────────────────

// CIFMessagesToOpenAI converts CIF messages to the OpenAI chat completions format.
func CIFMessagesToOpenAI(messages []cif.CIFMessage) []map[string]interface{} {
	var result []map[string]interface{}
	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			result = append(result, map[string]interface{}{
				"role":    "system",
				"content": m.Content,
			})
		case cif.CIFUserMessage:
			openaiMsg := map[string]interface{}{"role": "user"}
			if len(m.Content) == 1 {
				if textPart, ok := m.Content[0].(cif.CIFTextPart); ok {
					openaiMsg["content"] = textPart.Text
					result = append(result, openaiMsg)
					continue
				}
			}
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"type": "text", "text": p.Text})
				case cif.CIFToolResultPart:
					result = append(result, map[string]interface{}{
						"role":         "tool",
						"tool_call_id": p.ToolCallID,
						"content":      p.Content,
					})
					continue
				case cif.CIFImagePart:
					imgURL := map[string]interface{}{}
					if p.Data != nil {
						imgURL["url"] = fmt.Sprintf("data:%s;base64,%s", p.MediaType, *p.Data)
					} else if p.URL != nil {
						imgURL["url"] = *p.URL
					}
					parts = append(parts, map[string]interface{}{"type": "image_url", "image_url": imgURL})
				}
			}
			if len(parts) > 0 {
				openaiMsg["content"] = parts
				result = append(result, openaiMsg)
			}
		case cif.CIFAssistantMessage:
			openaiMsg := map[string]interface{}{"role": "assistant"}
			var textBuf strings.Builder
			var toolCalls []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBuf.WriteString(p.Text)
				case cif.CIFToolCallPart:
					args, _ := json.Marshal(p.ToolArguments)
					toolCalls = append(toolCalls, map[string]interface{}{
						"id":   p.ToolCallID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      p.ToolName,
							"arguments": string(args),
						},
					})
				}
			}
			if textBuf.Len() > 0 {
				openaiMsg["content"] = textBuf.String()
			}
			if len(toolCalls) > 0 {
				openaiMsg["tool_calls"] = toolCalls
			}
			result = append(result, openaiMsg)
		}
	}
	return result
}

// OpenAIRespToCIF converts an OpenAI chat completions response to CIF format.
func OpenAIRespToCIF(resp map[string]interface{}) *cif.CanonicalResponse {
	id, _ := resp["id"].(string)
	model, _ := resp["model"].(string)
	result := &cif.CanonicalResponse{ID: id, Model: model, StopReason: cif.StopReasonEndTurn}

	if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if fr, ok := choice["finish_reason"].(string); ok {
				result.StopReason = OpenAIStopReason(fr)
			}
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok && content != "" {
					result.Content = append(result.Content, cif.CIFTextPart{Type: "text", Text: content})
				}
				if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
					for _, tc := range toolCalls {
						tcMap, ok := tc.(map[string]interface{})
						if !ok {
							continue
						}
						if function, ok := tcMap["function"].(map[string]interface{}); ok {
							id, _ := tcMap["id"].(string)
							name, _ := function["name"].(string)
							args, _ := function["arguments"].(string)
							var toolArgs map[string]interface{}
							json.Unmarshal([]byte(args), &toolArgs) //nolint:errcheck
							result.Content = append(result.Content, cif.CIFToolCallPart{
								Type:          "tool_call",
								ToolCallID:    id,
								ToolName:      name,
								ToolArguments: toolArgs,
							})
						}
					}
				}
			}
		}
	}

	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		pt, _ := usage["prompt_tokens"].(float64)
		ct, _ := usage["completion_tokens"].(float64)
		result.Usage = &cif.CIFUsage{InputTokens: int(pt), OutputTokens: int(ct)}
	}

	return result
}

// OpenAIStopReason converts an OpenAI finish_reason to a CIF stop reason.
func OpenAIStopReason(reason string) cif.CIFStopReason {
	switch reason {
	case "stop":
		return cif.StopReasonEndTurn
	case "length":
		return cif.StopReasonMaxTokens
	case "tool_calls":
		return cif.StopReasonToolUse
	case "content_filter":
		return cif.StopReasonContentFilter
	default:
		return cif.StopReasonEndTurn
	}
}

// ParseOpenAISSE parses an OpenAI-compatible SSE stream into CIF events.
func ParseOpenAISSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var streamStartSent bool
	var contentBlockIndex int

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
			return
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Warn().Err(err).Msg("Failed to parse OpenAI SSE chunk")
			continue
		}

		if !streamStartSent {
			id, _ := chunk["id"].(string)
			model, _ := chunk["model"].(string)
			eventCh <- cif.CIFStreamStart{Type: "stream_start", ID: id, Model: model}
			streamStartSent = true
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}
		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
			var usage *cif.CIFUsage
			if usageMap, ok := chunk["usage"].(map[string]interface{}); ok {
				promptTokens, _ := usageMap["prompt_tokens"].(float64)
				completionTokens, _ := usageMap["completion_tokens"].(float64)
				usage = &cif.CIFUsage{
					InputTokens:  int(promptTokens),
					OutputTokens: int(completionTokens),
				}
			}
			eventCh <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: OpenAIStopReason(finishReason),
				Usage:      usage,
			}
			return
		}

		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		if content, ok := delta["content"].(string); ok && content != "" {
			eventCh <- cif.CIFContentDelta{
				Type:         "content_delta",
				Index:        contentBlockIndex,
				ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
				Delta:        cif.TextDelta{Type: "text_delta", Text: content},
			}
		}

		if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
			for _, tc := range toolCalls {
				tcMap, ok := tc.(map[string]interface{})
				if !ok {
					continue
				}
				if id, ok := tcMap["id"].(string); ok && id != "" {
					contentBlockIndex++
					funcMap, _ := tcMap["function"].(map[string]interface{})
					name, _ := funcMap["name"].(string)
					eventCh <- cif.CIFContentDelta{
						Type:  "content_delta",
						Index: contentBlockIndex,
						ContentBlock: cif.CIFToolCallPart{
							Type:          "tool_call",
							ToolCallID:    id,
							ToolName:      name,
							ToolArguments: map[string]interface{}{},
						},
						Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""},
					}
				} else if funcMap, ok := tcMap["function"].(map[string]interface{}); ok {
					if args, ok := funcMap["arguments"].(string); ok && args != "" {
						eventCh <- cif.CIFContentDelta{
							Type:  "content_delta",
							Index: contentBlockIndex,
							Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: args},
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Str("provider", "openai-compat").Msg("SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: "stream_error", Message: err.Error()},
		}
	}
}

// ─── CIF → Gemini message conversion ─────────────────────────────────────────

// CIFMessagesToGemini converts CIF messages to the Google Gemini contents format.
func CIFMessagesToGemini(messages []cif.CIFMessage) []map[string]interface{} {
	var contents []map[string]interface{}
	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			// System messages are handled via systemInstruction; skip here
			_ = m
		case cif.CIFUserMessage:
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"text": p.Text})
				case cif.CIFToolResultPart:
					parts = append(parts, map[string]interface{}{
						"functionResponse": map[string]interface{}{
							"name":     p.ToolName,
							"response": map[string]interface{}{"output": p.Content},
						},
					})
				case cif.CIFImagePart:
					if p.Data != nil {
						parts = append(parts, map[string]interface{}{
							"inlineData": map[string]interface{}{
								"mimeType": p.MediaType,
								"data":     *p.Data,
							},
						})
					}
				}
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]interface{}{"role": "user", "parts": parts})
			}
		case cif.CIFAssistantMessage:
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"text": p.Text})
				case cif.CIFToolCallPart:
					parts = append(parts, map[string]interface{}{
						"functionCall": map[string]interface{}{
							"name": p.ToolName,
							"args": p.ToolArguments,
						},
					})
				case cif.CIFThinkingPart:
					parts = append(parts, map[string]interface{}{"text": p.Thinking})
				}
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]interface{}{"role": "model", "parts": parts})
			}
		}
	}
	return contents
}

// SanitizeGeminiSchema removes fields that Gemini rejects from JSON Schema objects.
func SanitizeGeminiSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	blocked := map[string]bool{
		"$schema": true, "$id": true, "patternProperties": true, "prefill": true,
		"enumTitles": true, "deprecated": true, "propertyNames": true,
		"exclusiveMinimum": true, "exclusiveMaximum": true, "const": true,
	}
	clean := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		if blocked[k] {
			continue
		}
		switch nested := v.(type) {
		case map[string]interface{}:
			clean[k] = SanitizeGeminiSchema(nested)
		case []interface{}:
			cleaned := make([]interface{}, 0, len(nested))
			for _, item := range nested {
				if m, ok := item.(map[string]interface{}); ok {
					cleaned = append(cleaned, SanitizeGeminiSchema(m))
				} else {
					cleaned = append(cleaned, item)
				}
			}
			clean[k] = cleaned
		default:
			clean[k] = v
		}
	}
	return clean
}

// ─── Tool argument helpers ────────────────────────────────────────────────────

// NormalizeToolArguments converts arbitrary raw tool args to map[string]interface{}.
func NormalizeToolArguments(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case nil:
		return map[string]interface{}{}
	case map[string]interface{}:
		if value == nil {
			return map[string]interface{}{}
		}
		return value
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return map[string]interface{}{}
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && parsed != nil {
			return parsed
		}
		return map[string]interface{}{"value": value}
	case []interface{}:
		return map[string]interface{}{"items": value}
	default:
		return map[string]interface{}{"value": value}
	}
}

// ConvertCanonicalToolChoiceToOpenAI converts a CIF tool choice to OpenAI format.
func ConvertCanonicalToolChoiceToOpenAI(toolChoice interface{}) interface{} {
	switch choice := toolChoice.(type) {
	case string:
		switch choice {
		case "none", "auto", "required":
			return choice
		default:
			return nil
		}
	case map[string]interface{}:
		functionName, _ := choice["functionName"].(string)
		if functionName == "" {
			if function, ok := choice["function"].(map[string]interface{}); ok {
				functionName, _ = function["name"].(string)
			}
		}
		if functionName == "" {
			return nil
		}
		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": functionName,
			},
		}
	default:
		return nil
	}
}

// ─── Stream collection helper ─────────────────────────────────────────────────

// CollectStream assembles a CanonicalResponse from a CIF stream channel.
func CollectStream(ch <-chan cif.CIFStreamEvent) (*cif.CanonicalResponse, error) {
	response := &cif.CanonicalResponse{StopReason: cif.StopReasonEndTurn}
	var textBuf strings.Builder
	toolCalls := make(map[int]*cif.CIFToolCallPart)
	toolArgBufs := make(map[int]*strings.Builder)

	for event := range ch {
		switch e := event.(type) {
		case cif.CIFStreamStart:
			response.ID = e.ID
			response.Model = e.Model
		case cif.CIFContentDelta:
			switch d := e.Delta.(type) {
			case cif.TextDelta:
				textBuf.WriteString(d.Text)
			case cif.ToolArgumentsDelta:
				if toolArgBufs[e.Index] == nil {
					toolArgBufs[e.Index] = &strings.Builder{}
				}
				toolArgBufs[e.Index].WriteString(d.PartialJSON)
				if e.ContentBlock != nil {
					if tc, ok := e.ContentBlock.(cif.CIFToolCallPart); ok {
						toolCalls[e.Index] = &tc
					}
				}
			}
		case cif.CIFStreamEnd:
			response.StopReason = e.StopReason
			response.Usage = e.Usage
		case cif.CIFStreamError:
			return nil, fmt.Errorf("stream error: %s", e.Error.Message)
		}
	}

	if textBuf.Len() > 0 {
		response.Content = append(response.Content, cif.CIFTextPart{Type: "text", Text: textBuf.String()})
	}
	for idx, tc := range toolCalls {
		finalTC := *tc
		if buf, ok := toolArgBufs[idx]; ok {
			json.Unmarshal([]byte(buf.String()), &finalTC.ToolArguments) //nolint:errcheck
		}
		response.Content = append(response.Content, finalTC)
	}

	return response, nil
}

// ─── Misc helpers ─────────────────────────────────────────────────────────────

// RandomID generates a random hexadecimal ID string.
func RandomID() string {
	return fmt.Sprintf("%x%x", time.Now().UnixNano(), rand.Int63())
}

// FirstString returns the first non-empty string value for the given keys in a map.
func FirstString(values map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value, true
		}
	}
	return "", false
}

// ShortTokenSuffix returns the last 5 characters of a token for display purposes.
func ShortTokenSuffix(token string) string {
	trimmed := strings.TrimSpace(token)
	if len(trimmed) >= 5 {
		return trimmed[len(trimmed)-5:]
	}
	if trimmed == "" {
		return "token"
	}
	return trimmed
}
