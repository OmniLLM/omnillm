package generic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"omnimodel/internal/cif"
	alibabapkg "omnimodel/internal/providers/alibaba"
	"omnimodel/internal/providers/shared"

	"github.com/rs/zerolog/log"
)

const (
	defaultAlibabaAnthropicVersion   = "2023-06-01"
	defaultAlibabaAnthropicMaxTokens = 1024
)

type anthropicResponse struct {
	ID           string                  `json:"id"`
	Model        string                  `json:"model"`
	Content      []anthropicContentBlock `json:"content"`
	StopReason   *string                 `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        *anthropicResponseUsage `json:"usage"`
}

type anthropicResponseUsage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}

type anthropicContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Thinking  string                 `json:"thinking,omitempty"`
	Signature *string                `json:"signature,omitempty"`
	ID        string                 `json:"id,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
}

type anthropicStreamBlockState struct {
	blockType string
	toolCall  *cif.CIFToolCallPart
	sawDelta  bool
}

func (a *GenericAdapter) executeAnthropic(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload := a.buildAnthropicPayload(request)
	payload["stream"] = false
	return executeAnthropicWithPayload(alibabapkg.AnthropicMessagesURL(a.provider.baseURL), a.anthropicHeaders(false), payload, request.IncomingHeaders)
}

func (a *GenericAdapter) streamAnthropic(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	payload := a.buildAnthropicPayload(request)
	payload["stream"] = true
	return streamAnthropicWithPayload(alibabapkg.AnthropicMessagesURL(a.provider.baseURL), a.anthropicHeaders(true), payload, request.IncomingHeaders)
}

func (a *GenericAdapter) anthropicHeaders(stream bool) map[string]string {
	headers := a.provider.alibabaHeaders(stream)
	headers["Content-Type"] = "application/json"
	headers["anthropic-version"] = defaultAlibabaAnthropicVersion
	return headers
}

func (a *GenericAdapter) buildAnthropicPayload(request *cif.CanonicalRequest) map[string]interface{} {
	payload := map[string]interface{}{
		"model":    a.RemapModel(request.Model),
		"messages": cifMessagesToAnthropic(request.Messages),
	}

	if systemPrompt := anthropicSystemPrompt(request); systemPrompt != nil {
		payload["system"] = *systemPrompt
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
	} else {
		payload["max_tokens"] = defaultAlibabaAnthropicMaxTokens
	}
	if len(request.Stop) > 0 {
		payload["stop_sequences"] = request.Stop
	}
	if request.UserID != nil && strings.TrimSpace(*request.UserID) != "" {
		payload["metadata"] = map[string]interface{}{
			"user_id": *request.UserID,
		}
	}
	if len(request.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			entry := map[string]interface{}{
				"name":         tool.Name,
				"input_schema": tool.ParametersSchema,
			}
			if tool.Description != nil && strings.TrimSpace(*tool.Description) != "" {
				entry["description"] = *tool.Description
			}
			tools = append(tools, entry)
		}
		payload["tools"] = tools
	}
	if request.ToolChoice != nil {
		if toolChoice := convertCanonicalToolChoiceToAnthropic(request.ToolChoice); toolChoice != nil {
			payload["tool_choice"] = toolChoice
		}
	}

	return payload
}

func anthropicSystemPrompt(request *cif.CanonicalRequest) *string {
	var parts []string
	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		parts = append(parts, strings.TrimSpace(*request.SystemPrompt))
	}
	for _, message := range request.Messages {
		systemMessage, ok := message.(cif.CIFSystemMessage)
		if !ok {
			continue
		}
		if text := strings.TrimSpace(systemMessage.Content); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	joined := strings.Join(parts, "\n\n")
	return &joined
}

func cifMessagesToAnthropic(messages []cif.CIFMessage) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(messages))

	for _, message := range messages {
		switch m := message.(type) {
		case cif.CIFSystemMessage:
			continue
		case cif.CIFUserMessage:
			content := make([]map[string]interface{}, 0, len(m.Content))
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					content = append(content, map[string]interface{}{
						"type": "text",
						"text": p.Text,
					})
				case cif.CIFImagePart:
					if block := anthropicImageBlock(p); block != nil {
						content = append(content, block)
					}
				case cif.CIFToolResultPart:
					block := map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": p.ToolCallID,
						"content":     p.Content,
					}
					if p.ToolName != "" {
						block["name"] = p.ToolName
					}
					if p.IsError != nil {
						block["is_error"] = *p.IsError
					}
					content = append(content, block)
				}
			}
			if len(content) == 0 {
				continue
			}
			result = append(result, map[string]interface{}{
				"role":    "user",
				"content": content,
			})
		case cif.CIFAssistantMessage:
			content := make([]map[string]interface{}, 0, len(m.Content))
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					content = append(content, map[string]interface{}{
						"type": "text",
						"text": p.Text,
					})
				case cif.CIFThinkingPart:
					block := map[string]interface{}{
						"type":     "thinking",
						"thinking": p.Thinking,
					}
					if p.Signature != nil && strings.TrimSpace(*p.Signature) != "" {
						block["signature"] = *p.Signature
					}
					content = append(content, block)
				case cif.CIFToolCallPart:
					content = append(content, map[string]interface{}{
						"type":  "tool_use",
						"id":    p.ToolCallID,
						"name":  p.ToolName,
						"input": shared.NormalizeToolArguments(p.ToolArguments),
					})
				}
			}
			if len(content) == 0 {
				continue
			}
			result = append(result, map[string]interface{}{
				"role":    "assistant",
				"content": content,
			})
		}
	}

	return result
}

func anthropicImageBlock(part cif.CIFImagePart) map[string]interface{} {
	block := map[string]interface{}{
		"type": "image",
	}

	if part.Data != nil && *part.Data != "" {
		block["source"] = map[string]interface{}{
			"type":       "base64",
			"media_type": part.MediaType,
			"data":       *part.Data,
		}
		return block
	}

	if part.URL == nil || strings.TrimSpace(*part.URL) == "" {
		return nil
	}

	if mediaType, data, ok := parseDataURL(*part.URL); ok {
		block["source"] = map[string]interface{}{
			"type":       "base64",
			"media_type": mediaType,
			"data":       data,
		}
		return block
	}

	block["source"] = map[string]interface{}{
		"type": "url",
		"url":  *part.URL,
	}
	return block
}

func parseDataURL(raw string) (mediaType string, data string, ok bool) {
	if !strings.HasPrefix(raw, "data:") {
		return "", "", false
	}
	parts := strings.SplitN(raw, ",", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	header := strings.TrimPrefix(parts[0], "data:")
	data = parts[1]
	if header == "" || !strings.Contains(header, ";base64") {
		return "", "", false
	}
	mediaType = strings.TrimSuffix(header, ";base64")
	if mediaType == "" || data == "" {
		return "", "", false
	}
	return mediaType, data, true
}

func convertCanonicalToolChoiceToAnthropic(toolChoice interface{}) interface{} {
	switch choice := toolChoice.(type) {
	case string:
		switch choice {
		case "auto":
			return map[string]interface{}{"type": "auto"}
		case "required":
			return map[string]interface{}{"type": "any"}
		case "none":
			return map[string]interface{}{"type": "none"}
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
			"type": "tool",
			"name": functionName,
		}
	default:
		return nil
	}
}

func executeAnthropicWithPayload(url string, headers map[string]string, payload map[string]interface{}, incomingHeaders map[string]string) (*cif.CanonicalResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).RawJSON("incoming_headers", convertHeadersToJSON(incomingHeaders)).Msg("outbound anthropic proxy request payload")

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := genericHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to decode anthropic response: %w", err)
	}

	return anthropicRespToCIF(&anthropicResp), nil
}

func anthropicRespToCIF(resp *anthropicResponse) *cif.CanonicalResponse {
	result := &cif.CanonicalResponse{
		ID:         resp.ID,
		Model:      resp.Model,
		StopReason: cif.StopReasonEndTurn,
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content = append(result.Content, cif.CIFTextPart{
				Type: "text",
				Text: block.Text,
			})
		case "thinking":
			result.Content = append(result.Content, cif.CIFThinkingPart{
				Type:      "thinking",
				Thinking:  block.Thinking,
				Signature: block.Signature,
			})
		case "tool_use":
			toolCallID := block.ID
			if toolCallID == "" {
				toolCallID = block.ToolUseID
			}
			result.Content = append(result.Content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    toolCallID,
				ToolName:      block.Name,
				ToolArguments: shared.NormalizeToolArguments(block.Input),
			})
		}
	}

	if resp.StopReason != nil {
		result.StopReason = anthropicStopReason(*resp.StopReason)
	}
	result.StopSequence = resp.StopSequence

	if resp.Usage != nil {
		result.Usage = &cif.CIFUsage{
			InputTokens:           resp.Usage.InputTokens,
			OutputTokens:          resp.Usage.OutputTokens,
			CacheWriteInputTokens: resp.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:  resp.Usage.CacheReadInputTokens,
		}
	}

	return result
}

func anthropicStopReason(reason string) cif.CIFStopReason {
	switch reason {
	case "end_turn":
		return cif.StopReasonEndTurn
	case "max_tokens":
		return cif.StopReasonMaxTokens
	case "tool_use":
		return cif.StopReasonToolUse
	case "stop_sequence":
		return cif.StopReasonStopSequence
	case "content_filter":
		return cif.StopReasonContentFilter
	case "error":
		return cif.StopReasonError
	default:
		return cif.StopReasonEndTurn
	}
}

func convertHeadersToJSON(headers map[string]string) []byte {
	if len(headers) == 0 {
		return []byte("{}")
	}
	data, _ := json.Marshal(headers)
	return data
}

func streamAnthropicWithPayload(url string, headers map[string]string, payload map[string]interface{}, incomingHeaders map[string]string) (<-chan cif.CIFStreamEvent, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).RawJSON("incoming_headers", convertHeadersToJSON(incomingHeaders)).Msg("outbound anthropic proxy request payload")

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := genericStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("streaming API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parseAnthropicSSE(resp.Body, eventCh)
	return eventCh, nil
}

func parseAnthropicSSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var currentEvent string
	blocks := map[int]*anthropicStreamBlockState{}
	var inputTokens int
	var outputTokens int
	var cacheWriteTokens *int
	var cacheReadTokens *int
	var stopReason = cif.StopReasonEndTurn
	var stopSequence *string
	var hasUsage bool
	var ended bool

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, "data: "):
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			if payload == "" {
				continue
			}

			switch currentEvent {
			case "ping":
				continue
			case "message_start":
				var start struct {
					Message struct {
						ID    string `json:"id"`
						Model string `json:"model"`
						Usage *struct {
							InputTokens              int  `json:"input_tokens"`
							CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
							CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
						} `json:"usage"`
					} `json:"message"`
				}
				if err := json.Unmarshal([]byte(payload), &start); err != nil {
					log.Warn().Err(err).Msg("Failed to parse Anthropic message_start event")
					continue
				}
				if start.Message.Usage != nil {
					inputTokens = start.Message.Usage.InputTokens
					cacheWriteTokens = start.Message.Usage.CacheCreationInputTokens
					cacheReadTokens = start.Message.Usage.CacheReadInputTokens
					hasUsage = true
				}
				eventCh <- cif.CIFStreamStart{
					Type:  "stream_start",
					ID:    start.Message.ID,
					Model: start.Message.Model,
				}
			case "content_block_start":
				var start struct {
					Index        int `json:"index"`
					ContentBlock struct {
						Type  string                 `json:"type"`
						ID    string                 `json:"id,omitempty"`
						Name  string                 `json:"name,omitempty"`
						Input map[string]interface{} `json:"input,omitempty"`
					} `json:"content_block"`
				}
				if err := json.Unmarshal([]byte(payload), &start); err != nil {
					log.Warn().Err(err).Msg("Failed to parse Anthropic content_block_start event")
					continue
				}
				state := &anthropicStreamBlockState{
					blockType: start.ContentBlock.Type,
				}
				if start.ContentBlock.Type == "tool_use" {
					toolCall := cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    start.ContentBlock.ID,
						ToolName:      start.ContentBlock.Name,
						ToolArguments: shared.NormalizeToolArguments(start.ContentBlock.Input),
					}
					state.toolCall = &toolCall
					state.sawDelta = true
					eventCh <- cif.CIFContentDelta{
						Type:         "content_delta",
						Index:        start.Index,
						ContentBlock: toolCall,
						Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""},
					}
				}
				blocks[start.Index] = state
			case "content_block_delta":
				var delta struct {
					Index int `json:"index"`
					Delta struct {
						Type        string `json:"type"`
						Text        string `json:"text,omitempty"`
						Thinking    string `json:"thinking,omitempty"`
						PartialJSON string `json:"partial_json,omitempty"`
					} `json:"delta"`
				}
				if err := json.Unmarshal([]byte(payload), &delta); err != nil {
					log.Warn().Err(err).Msg("Failed to parse Anthropic content_block_delta event")
					continue
				}
				state, ok := blocks[delta.Index]
				if !ok {
					state = &anthropicStreamBlockState{}
					blocks[delta.Index] = state
				}
				var contentBlock cif.CIFContentPart
				if !state.sawDelta {
					switch state.blockType {
					case "text":
						contentBlock = cif.CIFTextPart{Type: "text", Text: ""}
					case "thinking":
						contentBlock = cif.CIFThinkingPart{Type: "thinking", Thinking: ""}
					case "tool_use":
						if state.toolCall != nil {
							contentBlock = *state.toolCall
						}
					}
				}
				switch delta.Delta.Type {
				case "text_delta":
					eventCh <- cif.CIFContentDelta{
						Type:         "content_delta",
						Index:        delta.Index,
						ContentBlock: contentBlock,
						Delta:        cif.TextDelta{Type: "text_delta", Text: delta.Delta.Text},
					}
					state.sawDelta = true
				case "thinking_delta":
					eventCh <- cif.CIFContentDelta{
						Type:         "content_delta",
						Index:        delta.Index,
						ContentBlock: contentBlock,
						Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: delta.Delta.Thinking},
					}
					state.sawDelta = true
				case "input_json_delta":
					eventCh <- cif.CIFContentDelta{
						Type:         "content_delta",
						Index:        delta.Index,
						ContentBlock: contentBlock,
						Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: delta.Delta.PartialJSON},
					}
					state.sawDelta = true
				}
			case "content_block_stop":
				var stop struct {
					Index int `json:"index"`
				}
				if err := json.Unmarshal([]byte(payload), &stop); err != nil {
					log.Warn().Err(err).Msg("Failed to parse Anthropic content_block_stop event")
					continue
				}
				eventCh <- cif.CIFContentBlockStop{
					Type:  "content_block_stop",
					Index: stop.Index,
				}
			case "message_delta":
				var delta struct {
					Delta struct {
						StopReason   *string `json:"stop_reason"`
						StopSequence *string `json:"stop_sequence"`
					} `json:"delta"`
					Usage *struct {
						OutputTokens             int  `json:"output_tokens"`
						CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
						CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
					} `json:"usage"`
				}
				if err := json.Unmarshal([]byte(payload), &delta); err != nil {
					log.Warn().Err(err).Msg("Failed to parse Anthropic message_delta event")
					continue
				}
				if delta.Delta.StopReason != nil {
					stopReason = anthropicStopReason(*delta.Delta.StopReason)
				}
				stopSequence = delta.Delta.StopSequence
				if delta.Usage != nil {
					outputTokens = delta.Usage.OutputTokens
					if delta.Usage.CacheCreationInputTokens != nil {
						cacheWriteTokens = delta.Usage.CacheCreationInputTokens
					}
					if delta.Usage.CacheReadInputTokens != nil {
						cacheReadTokens = delta.Usage.CacheReadInputTokens
					}
					hasUsage = true
				}
			case "message_stop":
				var usage *cif.CIFUsage
				if hasUsage {
					usage = &cif.CIFUsage{
						InputTokens:           inputTokens,
						OutputTokens:          outputTokens,
						CacheWriteInputTokens: cacheWriteTokens,
						CacheReadInputTokens:  cacheReadTokens,
					}
				}
				eventCh <- cif.CIFStreamEnd{
					Type:         "stream_end",
					StopReason:   stopReason,
					StopSequence: stopSequence,
					Usage:        usage,
				}
				ended = true
				return
			case "error":
				var evt struct {
					Error struct {
						Type    string `json:"type"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal([]byte(payload), &evt); err != nil {
					log.Warn().Err(err).Msg("Failed to parse Anthropic error event")
					continue
				}
				eventCh <- cif.CIFStreamError{
					Type: "stream_error",
					Error: cif.ErrorInfo{
						Type:    evt.Error.Type,
						Message: evt.Error.Message,
					},
				}
				ended = true
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Str("provider", "anthropic-compat").Msg("SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type: "stream_error",
			Error: cif.ErrorInfo{
				Type:    "stream_error",
				Message: err.Error(),
			},
		}
		return
	}

	if !ended {
		var usage *cif.CIFUsage
		if hasUsage {
			usage = &cif.CIFUsage{
				InputTokens:           inputTokens,
				OutputTokens:          outputTokens,
				CacheWriteInputTokens: cacheWriteTokens,
				CacheReadInputTokens:  cacheReadTokens,
			}
		}
		eventCh <- cif.CIFStreamEnd{
			Type:         "stream_end",
			StopReason:   stopReason,
			StopSequence: stopSequence,
			Usage:        usage,
		}
	}
}
