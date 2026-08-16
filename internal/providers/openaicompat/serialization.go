// CIF ↔ OpenAI-compatible wire serialization.
//
// BuildChatRequest converts a CIF CanonicalRequest to an OpenAI ChatRequest.
// ParseChatResponse converts a non-streaming ChatResponse to CIF.
// ParseSSE parses an SSE stream and emits CIF events on a channel.
//
// Provider-specific quirks (e.g. Qwen3 reasoning_content, enable_thinking) are
// injected via the Config passed to BuildChatRequest rather than living here.
package openaicompat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type PromptCacheMode string

const (
	PromptCacheDisabled        PromptCacheMode = "disabled"
	PromptCacheOpenAINative    PromptCacheMode = "openai_native"
	PromptCacheAnthropicInline PromptCacheMode = "anthropic_inline"
)

// Config carries per-provider knobs that affect how the request is built.
type Config struct {
	// DefaultTemperature / DefaultTopP are used when the caller omits sampling
	// parameters.  Zero values mean "omit the field".
	DefaultTemperature *float64
	DefaultTopP        *float64

	// IncludeUsageInStream requests per-chunk usage stats (stream_options).
	IncludeUsageInStream bool
	PromptCacheMode      PromptCacheMode

	// Extras are additional top-level JSON fields merged into the request body
	// (e.g. {"enable_thinking": true} for Qwen3).
	Extras map[string]interface{}
}

// BuildChatRequest converts a CIF CanonicalRequest into an OpenAI ChatRequest.
// model must already be the remapped provider model ID.
// stream controls whether stream=true is set.
func BuildChatRequest(model string, request *cif.CanonicalRequest, stream bool, cfg Config) (*ChatRequest, error) {
	messages, err := cifMessagesToOpenAI(request, cfg.PromptCacheMode == PromptCacheAnthropicInline)
	if err != nil {
		return nil, err
	}

	cr := &ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
	}

	// Sampling parameters.
	if request.Temperature != nil {
		cr.Temperature = request.Temperature
	} else if cfg.DefaultTemperature != nil {
		cr.Temperature = cfg.DefaultTemperature
	}
	if request.TopP != nil {
		cr.TopP = request.TopP
	} else if cfg.DefaultTopP != nil {
		cr.TopP = cfg.DefaultTopP
	}
	if request.MaxTokens != nil {
		cr.MaxTokens = request.MaxTokens
	}
	if len(request.Stop) > 0 {
		cr.Stop = request.Stop
	}
	if request.UserID != nil {
		userID := shared.TruncateOpenAIUserID(*request.UserID)
		cr.User = &userID
	}

	// Tools.
	if len(request.Tools) > 0 {
		cr.Tools = make([]Tool, 0, len(request.Tools))
		for _, t := range request.Tools {
			tool := Tool{
				Type: "function",
				Function: FunctionSpec{
					Name:       t.Name,
					Parameters: shared.NormalizeToolParameters(t.ParametersSchema),
				},
			}
			if t.Description != nil {
				tool.Function.Description = *t.Description
			}
			if cfg.PromptCacheMode == PromptCacheAnthropicInline {
				tool.CacheControl = t.CacheControl
			}
			cr.Tools = append(cr.Tools, tool)
		}
	}
	if request.ToolChoice != nil {
		cr.ToolChoice = shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice)
	}

	if request.PromptCache != nil {
		switch cfg.PromptCacheMode {
		case PromptCacheOpenAINative:
			cr.PromptCacheKey = request.PromptCache.Key
			cr.PromptCacheRetention = request.PromptCache.Retention
		case PromptCacheAnthropicInline:
			if request.PromptCache.Automatic != nil {
				if cr.Extras == nil {
					cr.Extras = make(map[string]interface{})
				}
				cr.Extras["cache_control"] = request.PromptCache.Automatic
			}
		}
	}

	if stream && cfg.IncludeUsageInStream {
		cr.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

	if cfg.Extras != nil {
		if cr.Extras == nil {
			cr.Extras = make(map[string]interface{}, len(cfg.Extras))
		}
		for key, value := range cfg.Extras {
			cr.Extras[key] = value
		}
	}
	return cr, nil
}

// Marshal serializes the ChatRequest to JSON, merging any Extras fields.
func Marshal(cr *ChatRequest) ([]byte, error) {
	// Build intermediate map to merge Extras.
	type alias ChatRequest
	base, err := json.Marshal((*alias)(cr))
	if err != nil {
		return nil, err
	}
	if len(cr.Extras) == 0 {
		return base, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	for k, v := range cr.Extras {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("openaicompat: failed to marshal extra field %q: %w", k, err)
		}
		m[k] = b
	}
	if userVal, ok := m["user"]; ok {
		var user string
		if err := json.Unmarshal(userVal, &user); err == nil {
			sanitized, err := json.Marshal(shared.TruncateOpenAIUserID(user))
			if err != nil {
				return nil, fmt.Errorf("openaicompat: failed to marshal sanitized user field: %w", err)
			}
			m["user"] = sanitized
		}
	}
	return json.Marshal(m)
}

// ─── CIF → OpenAI messages ────────────────────────────────────────────────────

func cifMessagesToOpenAI(request *cif.CanonicalRequest, inlineCache bool) ([]Message, error) {
	var msgs []Message

	if inlineCache {
		if len(request.System) > 0 {
			parts := make([]ContentPart, 0, len(request.System))
			for _, block := range request.System {
				parts = append(parts, ContentPart{Type: "text", Text: block.Text, CacheControl: block.CacheControl})
			}
			msgs = append(msgs, Message{Role: "system", Content: parts})
		}
	} else if system := cif.PlainSystemText(request.System); strings.TrimSpace(system) != "" {
		msgs = append(msgs, Message{Role: "system", Content: system})
	}

	for _, msg := range request.Messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			if inlineCache {
				parts := make([]ContentPart, 0, len(m.Content))
				for _, block := range m.Content {
					parts = append(parts, ContentPart{Type: "text", Text: block.Text, CacheControl: block.CacheControl})
				}
				msgs = append(msgs, Message{Role: "system", Content: parts})
			} else {
				msgs = append(msgs, Message{Role: "system", Content: cif.PlainSystemText(m.Content)})
			}
		case cif.CIFUserMessage:
			msgs = append(msgs, cifUserMsgs(m, inlineCache)...)
		case cif.CIFAssistantMessage:
			msgs = append(msgs, cifAssistantMsg(m, inlineCache))
		}
	}
	return msgs, nil
}

func cifUserMsgs(m cif.CIFUserMessage, inlineCache bool) []Message {
	var userParts []ContentPart
	var toolMsgs []Message

	for _, part := range m.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			part := ContentPart{Type: "text", Text: p.Text}
			if inlineCache {
				part.CacheControl = p.CacheControl
			}
			userParts = append(userParts, part)
		case cif.CIFImagePart:
			var url string
			if p.Data != nil {
				url = fmt.Sprintf("data:%s;base64,%s", p.MediaType, *p.Data)
			} else if p.URL != nil {
				url = *p.URL
			}
			imagePart := ContentPart{
				Type:     "image_url",
				ImageURL: &ImageURL{URL: url},
			}
			if inlineCache {
				imagePart.CacheControl = p.CacheControl
			}
			userParts = append(userParts, imagePart)
		case cif.CIFToolResultPart:
			content := p.Content
			if p.IsError != nil && *p.IsError && content == "" {
				content = "Error: tool call failed"
			}
			toolMessage := Message{
				Role:       "tool",
				Content:    content,
				ToolCallID: p.ToolCallID,
			}
			if inlineCache {
				toolMessage.CacheControl = p.CacheControl
			}
			toolMsgs = append(toolMsgs, toolMessage)
		}
	}

	var result []Message
	result = append(result, toolMsgs...)
	if len(userParts) > 0 {
		if !inlineCache && len(userParts) == 1 && userParts[0].Type == "text" {
			result = append(result, Message{Role: "user", Content: userParts[0].Text})
		} else {
			result = append(result, Message{Role: "user", Content: userParts})
		}
	}
	return result
}

func cifAssistantMsg(m cif.CIFAssistantMessage, inlineCache bool) Message {
	msg := Message{Role: "assistant"}
	var textBuf strings.Builder
	var contentParts []ContentPart
	var toolCalls []ToolCall
	var reasoningContent string

	for _, part := range m.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			if inlineCache {
				contentParts = append(contentParts, ContentPart{Type: "text", Text: p.Text, CacheControl: p.CacheControl})
			} else {
				textBuf.WriteString(p.Text)
			}
		case cif.CIFThinkingPart:
			reasoningContent = p.Thinking
		case cif.CIFToolCallPart:
			args, _ := json.Marshal(p.ToolArguments)
			call := ToolCall{
				ID:   p.ToolCallID,
				Type: "function",
				Function: FunctionCallSpec{
					Name:      p.ToolName,
					Arguments: string(args),
				},
			}
			if inlineCache {
				call.CacheControl = p.CacheControl
			}
			toolCalls = append(toolCalls, call)
		}
	}
	if textBuf.Len() > 0 {
		msg.Content = textBuf.String()
	}
	if len(contentParts) > 0 {
		msg.Content = contentParts
	}
	if reasoningContent != "" {
		msg.ReasoningContent = reasoningContent
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	return msg
}

// ─── Response → CIF ──────────────────────────────────────────────────────────

// ParseChatResponse converts a non-streaming ChatResponse to CIF.
func usageToCIF(usage *Usage) *cif.CIFUsage {
	if usage == nil {
		return nil
	}
	var cached *int
	if usage.PromptTokensDetails != nil {
		value := usage.PromptTokensDetails.CachedTokens
		cached = &value
	}
	return cif.UsageFromTotal(usage.PromptTokens, usage.CompletionTokens, cached)
}

func ParseChatResponse(resp *ChatResponse) *cif.CanonicalResponse {
	result := &cif.CanonicalResponse{
		ID:         resp.ID,
		Model:      resp.Model,
		StopReason: cif.StopReasonEndTurn,
	}
	if len(resp.Choices) > 0 {
		ch := resp.Choices[0]
		result.StopReason = StopReason(ch.FinishReason)
		if ch.Message.ReasoningContent != "" {
			result.Content = append(result.Content, cif.CIFThinkingPart{Type: "thinking", Thinking: ch.Message.ReasoningContent})
		}
		if text, ok := ch.Message.Content.(string); ok && text != "" {
			result.Content = append(result.Content, cif.CIFTextPart{Type: "text", Text: text})
		}
		for _, tc := range ch.Message.ToolCalls {
			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Function.Arguments), &args) //nolint:errcheck
			result.Content = append(result.Content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    tc.ID,
				ToolName:      tc.Function.Name,
				ToolArguments: args,
			})
		}
	}
	if resp.Usage != nil {
		var cached *int
		if resp.Usage.PromptTokensDetails != nil {
			cachedValue := resp.Usage.PromptTokensDetails.CachedTokens
			cached = &cachedValue
		}
		result.Usage = cif.UsageFromTotal(resp.Usage.PromptTokens, resp.Usage.CompletionTokens, cached)
	}
	return result
}

// StopReason maps an OpenAI finish_reason string to a CIF stop reason.
func StopReason(reason string) cif.CIFStopReason {
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

// ─── SSE parser ───────────────────────────────────────────────────────────────

// ParseSSE reads an OpenAI-compatible SSE stream and emits CIF events on
// eventCh.  The channel is closed when the stream ends or on error.
//
// Quirks handled:
//   - reasoning_content deltas → CIFThinkingPart / ThinkingDelta (Qwen3 / o1)
//   - finish_reason "stop" when tool calls were observed → upgraded to ToolUse
//   - tool_call.index continuations mapped across chunks
func ParseSSE(ctx context.Context, body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)
	// When the request context is cancelled (client disconnect), close the body
	// so the scanner's Read call unblocks and the goroutine exits cleanly.
	stop := context.AfterFunc(ctx, func() { body.Close() })
	defer stop()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4*1024), 1024*1024)

	var streamStartSent bool
	var contentBlockIndex int
	providerToolIndexToBlock := map[int]int{}
	toolCallsSeen := map[int]bool{}
	var thinkingBlockOpen bool
	const thinkingIdx = -1 // placed before text/tool blocks

	var pendingStop *cif.CIFStopReason
	var pendingUsage *cif.CIFUsage

	emitEnd := func() {
		stopReason := cif.StopReasonEndTurn
		if pendingStop != nil {
			stopReason = *pendingStop
		} else if len(toolCallsSeen) > 0 {
			stopReason = cif.StopReasonToolUse
		}
		eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: stopReason, Usage: pendingUsage}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			emitEnd()
			return
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Warn().Err(err).Msg("openaicompat: failed to parse SSE chunk")
			continue
		}

		if !streamStartSent {
			eventCh <- cif.CIFStreamStart{Type: "stream_start", ID: chunk.ID, Model: chunk.Model}
			streamStartSent = true
		}

		if len(chunk.Choices) == 0 {
			pendingUsage = usageToCIF(chunk.Usage)
			continue
		}
		choice := chunk.Choices[0]

		if choice.FinishReason != "" {
			stopReason := StopReason(choice.FinishReason)
			if stopReason != cif.StopReasonToolUse && len(toolCallsSeen) > 0 {
				stopReason = cif.StopReasonToolUse
			}
			pendingStop = &stopReason
			if chunk.Usage != nil {
				pendingUsage = usageToCIF(chunk.Usage)
			}
			continue
		}

		delta := choice.Delta

		// reasoning_content (Qwen3 thinking / o1 reasoning).
		if delta.ReasoningContent != "" {
			if !thinkingBlockOpen {
				eventCh <- cif.CIFContentDelta{
					Type:         "content_delta",
					Index:        thinkingIdx,
					ContentBlock: cif.CIFThinkingPart{Type: "thinking", Thinking: ""},
					Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: delta.ReasoningContent},
				}
				thinkingBlockOpen = true
			} else {
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: thinkingIdx,
					Delta: cif.ThinkingDelta{Type: "thinking_delta", Thinking: delta.ReasoningContent},
				}
			}
		}

		if delta.Content != "" {
			eventCh <- cif.CIFContentDelta{
				Type:         "content_delta",
				Index:        contentBlockIndex,
				ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
				Delta:        cif.TextDelta{Type: "text_delta", Text: delta.Content},
			}
		}

		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			// DashScope sends call_id instead of id; accept either.
			toolCallID := firstNonEmpty(tc.ID, tc.CallID)
			if toolCallID != "" {
				contentBlockIndex++
				providerToolIndexToBlock[idx] = contentBlockIndex
				toolCallsSeen[idx] = true
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: contentBlockIndex,
					ContentBlock: cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    toolCallID,
						ToolName:      tc.Function.Name,
						ToolArguments: map[string]interface{}{},
					},
					Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""},
				}
				// Some providers send complete args in the same chunk as the ID.
				if tc.Function.Arguments != "" {
					eventCh <- cif.CIFContentDelta{
						Type:  "content_delta",
						Index: contentBlockIndex,
						Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: tc.Function.Arguments},
					}
				}
			} else if tc.Function.Arguments != "" {
				blockIdx, exists := providerToolIndexToBlock[idx]
				if !exists {
					continue
				}
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: blockIdx,
					Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: tc.Function.Arguments},
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && !shared.IsBenignStreamEndError(ctx, err) {
		zerolog.Ctx(ctx).Error().Err(err).Str("provider", "openaicompat").Msg("SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: "stream_error", Message: err.Error()},
		}
	}
}
