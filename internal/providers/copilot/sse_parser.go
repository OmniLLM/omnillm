package copilot

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"omnillm/internal/cif"

	"github.com/rs/zerolog/log"
)

func (a *CopilotAdapter) parseOpenAISSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent, toolNameMapper *copilotToolNameMapper) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4*1024), 1024*1024)
	var streamStartSent bool
	var contentBlockIndex int
	providerToolIndexToBlock := map[int]int{}
	toolCallsSeen := map[int]bool{}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			stopReason := cif.StopReasonEndTurn
			if len(toolCallsSeen) > 0 {
				stopReason = cif.StopReasonToolUse
			}
			eventCh <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: stopReason,
			}
			return
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Warn().Err(err).Msg("Failed to parse SSE chunk")
			continue
		}

		if !streamStartSent {
			id, _ := chunk["id"].(string)
			model, _ := chunk["model"].(string)
			eventCh <- cif.CIFStreamStart{
				Type:  "stream_start",
				ID:    id,
				Model: model,
			}
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

			stopReason := a.convertOpenAIStopReason(finishReason)
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

		if content, ok := delta["content"].(string); ok && content != "" {
			eventCh <- cif.CIFContentDelta{
				Type:  "content_delta",
				Index: contentBlockIndex,
				ContentBlock: cif.CIFTextPart{
					Type: "text",
					Text: "",
				},
				Delta: cif.TextDelta{
					Type: "text_delta",
					Text: content,
				},
			}
		}

		if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
			for _, tc := range toolCalls {
				tcMap, ok := tc.(map[string]interface{})
				if !ok {
					continue
				}

				providerIdx := 0
				if idxRaw, ok := tcMap["index"].(float64); ok {
					providerIdx = int(idxRaw)
				}

				id, _ := tcMap["id"].(string)
				if id == "" {
					id, _ = tcMap["call_id"].(string)
				}

				if id != "" {
					contentBlockIndex++
					providerToolIndexToBlock[providerIdx] = contentBlockIndex
					toolCallsSeen[providerIdx] = true
					funcMap, _ := tcMap["function"].(map[string]interface{})
					name, _ := funcMap["name"].(string)

					eventCh <- cif.CIFContentDelta{
						Type:  "content_delta",
						Index: contentBlockIndex,
						ContentBlock: cif.CIFToolCallPart{
							Type:          "tool_call",
							ToolCallID:    id,
							ToolName:      toolNameMapper.fromUpstream(name),
							ToolArguments: map[string]interface{}{},
						},
						Delta: cif.ToolArgumentsDelta{
							Type:        "tool_arguments_delta",
							PartialJSON: "",
						},
					}
					if funcMap != nil {
						if args, ok := funcMap["arguments"].(string); ok && args != "" {
							eventCh <- cif.CIFContentDelta{
								Type:  "content_delta",
								Index: contentBlockIndex,
								Delta: cif.ToolArgumentsDelta{
									Type:        "tool_arguments_delta",
									PartialJSON: args,
								},
							}
						}
					}
				} else if funcMap, ok := tcMap["function"].(map[string]interface{}); ok {
					if args, ok := funcMap["arguments"].(string); ok && args != "" {
						blockIndex, exists := providerToolIndexToBlock[providerIdx]
						if !exists {
							continue
						}
						eventCh <- cif.CIFContentDelta{
							Type:  "content_delta",
							Index: blockIndex,
							Delta: cif.ToolArgumentsDelta{
								Type:        "tool_arguments_delta",
								PartialJSON: args,
							},
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Msg("SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type: "stream_error",
			Error: cif.ErrorInfo{
				Type:    "stream_error",
				Message: err.Error(),
			},
		}
	}
}
