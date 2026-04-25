package shared

import (
	"bufio"
	"encoding/json"
	"io"
	"omnillm/internal/cif"
	"strings"

	"github.com/rs/zerolog/log"
)

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
