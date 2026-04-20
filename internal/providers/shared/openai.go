// Package shared provides utilities shared across multiple provider implementations.
package shared

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"omnillm/internal/cif"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// ─── CIF → OpenAI message conversion ─────────────────────────────────────────

// nonEmptyStringPtr returns a pointer to s if s contains non-whitespace text,
// otherwise nil. This avoids repeating the "trim + take address" pattern
// across provider parsers.
func nonEmptyStringPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

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
					content := p.Content
					if p.IsError != nil && *p.IsError && content == "" {
						content = "Error: tool call failed"
					}
					result = append(result, map[string]interface{}{
						"role":         "tool",
						"tool_call_id": p.ToolCallID,
						"content":      content,
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
			var reasoningContent string
			var reasoningSignature *string
			var toolCalls []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBuf.WriteString(p.Text)
				case cif.CIFThinkingPart:
					// For OpenAI-compatible providers (DashScope Qwen), the thinking
					// from a prior turn is forwarded as reasoning_content so the model
					// can continue reasoning coherently in multi-turn conversations.
					reasoningContent = p.Thinking
					if p.Signature != nil {
						reasoningSignature = nonEmptyStringPtr(*p.Signature)
					}
				case cif.CIFToolCallPart:
					args, _ := json.Marshal(p.ToolArguments)
					toolCalls = append(toolCalls, map[string]interface{}{
						"id":      p.ToolCallID,
						"call_id": p.ToolCallID,
						"type":    "function",
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
			if reasoningContent != "" {
				openaiMsg["reasoning_content"] = reasoningContent
				if reasoningSignature != nil {
					openaiMsg["reasoning_signature"] = *reasoningSignature
				}
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

// NormalizeOpenAICompatibleAPIFormat canonicalizes supported OpenAI-compatible
// upstream API format aliases.
func NormalizeOpenAICompatibleAPIFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "responses", "response", "openai-responses", "openai_responses":
		return "responses"
	case "chat", "chat.completions", "chat_completions", "openai-chat", "openai_chat":
		return "chat.completions"
	default:
		return ""
	}
}

// ParseOpenAISSE parses an OpenAI-compatible SSE stream into CIF events.
//
// Qwen3/Alibaba quirks handled here:
//   - finish_reason may be "stop" even when tool calls were made; the stop
//     reason is overridden to StopReasonToolUse when any tool call deltas
//     were observed during the stream.
//   - reasoning_content in delta chunks (Qwen3 thinking) is forwarded as
//     ThinkingDelta events so the thinking is not silently dropped.
func ParseOpenAISSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4*1024), 1024*1024)

	var streamStartSent bool
	var contentBlockIndex int
	// providerToolIndexToContentIndex maps provider-side tool_call.index values to
	// the local CIF content block index allocated for that tool call. DashScope /
	// Qwen streams later argument deltas keyed only by provider index, so we must
	// preserve this mapping across chunks.
	providerToolIndexToContentIndex := map[int]int{}
	// toolCallsSeen tracks tool call blocks by their provider-side index so we
	// can correctly handle multi-tool streams and override the stop reason.
	toolCallsSeen := map[int]bool{}
	// thinkingBlockOpen tracks whether a thinking content block is currently
	// open (Qwen3 sends reasoning_content across many delta chunks).
	var thinkingBlockOpen bool
	const thinkingBlockIndex = -1 // sentinel: placed before text/tool blocks

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// No finish_reason was emitted before [DONE] — synthesise a stop
			// event using whatever we observed in the stream.
			stopReason := cif.StopReasonEndTurn
			if len(toolCallsSeen) > 0 {
				stopReason = cif.StopReasonToolUse
			}
			eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: stopReason}
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
			// Some providers (e.g. Qwen3) report finish_reason "stop" even
			// when the response contains tool calls.  If we observed any tool
			// call deltas during the stream, upgrade the stop reason so that
			// the caller knows it must execute the tools.
			stopReason := OpenAIStopReason(finishReason)
			if stopReason != cif.StopReasonToolUse && len(toolCallsSeen) > 0 {
				stopReason = cif.StopReasonToolUse
			}
			eventCh <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: stopReason,
				Usage:      usage,
			}
			return
		}

		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		// Handle Qwen3 reasoning_content (thinking) deltas.
		if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
			var signature *string
			if rawSig, ok := delta["reasoning_signature"].(string); ok {
				signature = nonEmptyStringPtr(rawSig)
			}
			if !thinkingBlockOpen {
				eventCh <- cif.CIFContentDelta{
					Type:         "content_delta",
					Index:        thinkingBlockIndex,
					ContentBlock: cif.CIFThinkingPart{Type: "thinking", Thinking: "", Signature: signature},
					Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: reasoning},
				}
				thinkingBlockOpen = true
			} else {
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: thinkingBlockIndex,
					Delta: cif.ThinkingDelta{Type: "thinking_delta", Thinking: reasoning},
				}
			}
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
				// Determine the provider-side index for this tool call chunk.
				providerIdx := 0
				if idxRaw, ok := tcMap["index"].(float64); ok {
					providerIdx = int(idxRaw)
				}

				if id, ok := tcMap["id"].(string); ok && id != "" {
					// New tool call: allocate a new content block index for it and
					// remember the mapping from provider index -> local block index.
					contentBlockIndex++
					providerToolIndexToContentIndex[providerIdx] = contentBlockIndex
					toolCallsSeen[providerIdx] = true
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
					// Some providers (e.g. GLM) send the complete arguments in the
					// same chunk as the tool call id rather than in a separate delta.
					// Emit an additional arguments delta so the arguments are not lost.
					if funcMap != nil {
						if args, ok := funcMap["arguments"].(string); ok && args != "" {
							eventCh <- cif.CIFContentDelta{
								Type:  "content_delta",
								Index: contentBlockIndex,
								Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: args},
							}
						}
					}
				} else if funcMap, ok := tcMap["function"].(map[string]interface{}); ok {
					// Continuation chunk: route arguments to the original content block
					// for this provider-side tool_call.index.
					blockIndex, exists := providerToolIndexToContentIndex[providerIdx]
					if !exists {
						continue
					}
					if args, ok := funcMap["arguments"].(string); ok && args != "" {
						eventCh <- cif.CIFContentDelta{
							Type:  "content_delta",
							Index: blockIndex,
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
	textParts := make(map[int]*strings.Builder)
	thinkingParts := make(map[int]*strings.Builder)
	thinkingSignatures := make(map[int]*string)
	toolCalls := make(map[int]*cif.CIFToolCallPart)
	toolArgBufs := make(map[int]*strings.Builder)

	for event := range ch {
		switch e := event.(type) {
		case cif.CIFStreamStart:
			response.ID = e.ID
			response.Model = e.Model
		case cif.CIFContentDelta:
			if e.ContentBlock != nil {
				switch cb := e.ContentBlock.(type) {
				case cif.CIFThinkingPart:
					if cb.Signature != nil {
						thinkingSignatures[e.Index] = cb.Signature
					}
				case cif.CIFToolCallPart:
					toolCopy := cb
					toolCalls[e.Index] = &toolCopy
				}
			}
			switch d := e.Delta.(type) {
			case cif.TextDelta:
				if textParts[e.Index] == nil {
					textParts[e.Index] = &strings.Builder{}
				}
				textParts[e.Index].WriteString(d.Text)
			case cif.ThinkingDelta:
				if thinkingParts[e.Index] == nil {
					thinkingParts[e.Index] = &strings.Builder{}
				}
				thinkingParts[e.Index].WriteString(d.Thinking)
			case cif.ToolArgumentsDelta:
				if toolArgBufs[e.Index] == nil {
					toolArgBufs[e.Index] = &strings.Builder{}
				}
				toolArgBufs[e.Index].WriteString(d.PartialJSON)
			}
		case cif.CIFStreamEnd:
			response.StopReason = e.StopReason
			response.Usage = e.Usage
		case cif.CIFStreamError:
			return nil, fmt.Errorf("stream error: %s", e.Error.Message)
		}
	}

	indicesSet := make(map[int]struct{})
	for idx := range thinkingParts {
		indicesSet[idx] = struct{}{}
	}
	for idx := range textParts {
		indicesSet[idx] = struct{}{}
	}
	for idx := range toolCalls {
		indicesSet[idx] = struct{}{}
	}
	indices := make([]int, 0, len(indicesSet))
	for idx := range indicesSet {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	for _, idx := range indices {
		if buf, ok := thinkingParts[idx]; ok && buf.Len() > 0 {
			response.Content = append(response.Content, cif.CIFThinkingPart{
				Type:      "thinking",
				Thinking:  buf.String(),
				Signature: thinkingSignatures[idx],
			})
		}
		if buf, ok := textParts[idx]; ok && buf.Len() > 0 {
			response.Content = append(response.Content, cif.CIFTextPart{Type: "text", Text: buf.String()})
		}
		if tc, ok := toolCalls[idx]; ok {
			finalTC := *tc
			if buf, ok := toolArgBufs[idx]; ok {
				json.Unmarshal([]byte(buf.String()), &finalTC.ToolArguments) //nolint:errcheck
			}
			response.Content = append(response.Content, finalTC)
		}
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

// NormalizeToolParameters ensures tool parameters are never nil.
// The OpenAI-compatible API (used by Qwen/DashScope, Azure, etc.) expects
// "parameters" to be a JSON Schema object, defaulting to {}. Serialising nil
// produces "parameters": null which some providers reject.
func NormalizeToolParameters(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}
	return schema
}
