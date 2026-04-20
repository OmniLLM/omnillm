package serialization

import (
	"encoding/json"
	"fmt"
	"time"

	"omnillm/internal/cif"
)

// Anthropic streaming types

type AnthropicStreamState struct {
	MessageStartSent      bool
	NextContentBlockIndex int
	ContentBlockOpen      bool
	CurrentBlockProviderIdx int
	CurrentBlockAnthropicIdx int
	CurrentBlockType      string
	// SuppressThinkingBlocks drops thinking content blocks from the stream.
	// This must be set when the client did not opt in to the
	// interleaved-thinking beta, otherwise the Anthropic SDK fails to parse
	// the response and silently stops processing (e.g. never executing a
	// tool_use block that follows a thinking block).
	SuppressThinkingBlocks bool
	// SuppressedProviderIdx is the provider-side index of a thinking block
	// currently being suppressed (set while SuppressThinkingBlocks is true).
	SuppressedProviderIdx int
}

func CreateAnthropicStreamState() *AnthropicStreamState {
	return &AnthropicStreamState{
		CurrentBlockProviderIdx:  -1,
		CurrentBlockAnthropicIdx: -1,
		SuppressedProviderIdx:    -1,
	}
}

// ConvertCIFEventToAnthropicSSE converts CIF stream events to Anthropic SSE events
func ConvertCIFEventToAnthropicSSE(event cif.CIFStreamEvent, state *AnthropicStreamState) ([]map[string]interface{}, error) {
	var events []map[string]interface{}

	switch e := event.(type) {
	case cif.CIFStreamStart:
		state.MessageStartSent = true
		state.NextContentBlockIndex = 0
		state.ContentBlockOpen = false
		state.CurrentBlockProviderIdx = -1
		state.CurrentBlockAnthropicIdx = -1
		state.CurrentBlockType = ""
		state.SuppressedProviderIdx = -1

		messageStart := map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":            e.ID,
				"type":          "message",
				"role":          "assistant",
				"model":         e.Model,
				"content":       []interface{}{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]interface{}{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		}
		events = append(events, messageStart)

	case cif.CIFContentDelta:
		// Drop thinking blocks entirely when the client did not opt in to the
		// interleaved-thinking beta. Forwarding unexpected thinking blocks to the
		// Anthropic SDK causes it to silently stop processing the stream, which
		// means any tool_use block that follows never gets executed.
		if state.SuppressThinkingBlocks {
			if e.ContentBlock != nil {
				if _, isThinking := e.ContentBlock.(cif.CIFThinkingPart); isThinking {
					state.SuppressedProviderIdx = e.Index
					return events, nil
				}
			}
			if state.SuppressedProviderIdx == e.Index {
				if _, isThinkingDelta := e.Delta.(cif.ThinkingDelta); isThinkingDelta {
					return events, nil
				}
				state.SuppressedProviderIdx = -1
			}
		}

		if e.ContentBlock != nil {
			blockType := getBlockType(e.ContentBlock)
			if blockType == "" {
				return events, nil
			}

			needsNewBlock := !state.ContentBlockOpen ||
				state.CurrentBlockProviderIdx != e.Index ||
				state.CurrentBlockType != blockType

			if needsNewBlock {
				if state.ContentBlockOpen {
					events = append(events, map[string]interface{}{
						"type":  "content_block_stop",
						"index": state.CurrentBlockAnthropicIdx,
					})
				}

				anthropicIdx := state.NextContentBlockIndex
				state.NextContentBlockIndex++

				startEvent := createContentBlockStartEvent(e.ContentBlock, anthropicIdx)
				if startEvent != nil {
					events = append(events, startEvent)
				}

				state.ContentBlockOpen = true
				state.CurrentBlockProviderIdx = e.Index
				state.CurrentBlockAnthropicIdx = anthropicIdx
				state.CurrentBlockType = blockType
			}
		}

		if !state.ContentBlockOpen {
			return events, nil
		}

		switch d := e.Delta.(type) {
		case cif.TextDelta:
			events = append(events, map[string]interface{}{
				"type":  "content_block_delta",
				"index": state.CurrentBlockAnthropicIdx,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": d.Text,
				},
			})
		case cif.ThinkingDelta:
			events = append(events, map[string]interface{}{
				"type":  "content_block_delta",
				"index": state.CurrentBlockAnthropicIdx,
				"delta": map[string]interface{}{
					"type":     "thinking_delta",
					"thinking": d.Thinking,
				},
			})
		case cif.ToolArgumentsDelta:
			events = append(events, map[string]interface{}{
				"type":  "content_block_delta",
				"index": state.CurrentBlockAnthropicIdx,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": d.PartialJSON,
				},
			})
		}

	case cif.CIFContentBlockStop:
		if state.ContentBlockOpen {
			events = append(events, map[string]interface{}{
				"type":  "content_block_stop",
				"index": state.CurrentBlockAnthropicIdx,
			})
			state.ContentBlockOpen = false
			state.CurrentBlockProviderIdx = -1
			state.CurrentBlockAnthropicIdx = -1
			state.CurrentBlockType = ""
		}

	case cif.CIFStreamEnd:
		if state.ContentBlockOpen {
			events = append(events, map[string]interface{}{
				"type":  "content_block_stop",
				"index": state.CurrentBlockAnthropicIdx,
			})
			state.ContentBlockOpen = false
		}

		messageDelta := map[string]interface{}{
			"type": "message_delta",
			"delta": map[string]interface{}{
				"stop_reason":   convertStopReasonToAnthropic(e.StopReason),
				"stop_sequence": e.StopSequence,
			},
		}
		if e.Usage != nil {
			messageDelta["usage"] = map[string]interface{}{
				"output_tokens": e.Usage.OutputTokens,
			}
			if e.Usage.CacheWriteInputTokens != nil {
				messageDelta["usage"].(map[string]interface{})["cache_creation_input_tokens"] = *e.Usage.CacheWriteInputTokens
			}
			if e.Usage.CacheReadInputTokens != nil {
				messageDelta["usage"].(map[string]interface{})["cache_read_input_tokens"] = *e.Usage.CacheReadInputTokens
			}
		}
		events = append(events, messageDelta)
		events = append(events, map[string]interface{}{"type": "message_stop"})

	case cif.CIFStreamError:
		events = append(events, map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    e.Error.Type,
				"message": e.Error.Message,
			},
		})
	}

	return events, nil
}

func getBlockType(contentBlock cif.CIFContentPart) string {
	switch contentBlock.(type) {
	case cif.CIFTextPart:
		return "text"
	case cif.CIFThinkingPart:
		return "thinking"
	case cif.CIFToolCallPart:
		return "tool_call"
	default:
		return ""
	}
}

func createContentBlockStartEvent(contentBlock cif.CIFContentPart, index int) map[string]interface{} {
	switch cb := contentBlock.(type) {
	case cif.CIFTextPart:
		return map[string]interface{}{
			"type":  "content_block_start",
			"index": index,
			"content_block": map[string]interface{}{
				"type": "text",
				"text": "",
			},
		}
	case cif.CIFThinkingPart:
		return map[string]interface{}{
			"type":  "content_block_start",
			"index": index,
			"content_block": map[string]interface{}{
				"type":     "thinking",
				"thinking": "",
			},
		}
	case cif.CIFToolCallPart:
		return map[string]interface{}{
			"type":  "content_block_start",
			"index": index,
			"content_block": map[string]interface{}{
				"type":  "tool_use",
				"id":    cb.ToolCallID,
				"name":  cb.ToolName,
				"input": map[string]interface{}{},
			},
		}
	default:
		return nil
	}
}

// FormatAnthropicSSEData formats data as Anthropic SSE event
func FormatAnthropicSSEData(eventType string, data interface{}) (string, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(jsonBytes)), nil
}

// Responses API serialization types

type ResponsesResponse struct {
	ID        string                 `json:"id"`
	Object    string                 `json:"object"`
	Model     string                 `json:"model"`
	Output    []ResponsesOutputItem  `json:"output"`
	Usage     *ResponsesUsage        `json:"usage,omitempty"`
	CreatedAt int64                  `json:"created_at,omitempty"`
}

type ResponsesOutputItem struct {
	Type      string                  `json:"type"`
	ID        string                  `json:"id"`
	Role      string                  `json:"role"`
	Content   []ResponsesContentBlock `json:"content,omitempty"`
	Name      string                  `json:"name,omitempty"`
	Arguments string                  `json:"arguments,omitempty"`
}

type ResponsesContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// SerializeToResponses converts a CIF response to Responses API format
func SerializeToResponses(response *cif.CanonicalResponse) (*ResponsesResponse, error) {
	var outputItems []ResponsesOutputItem
	var contentBlocks []ResponsesContentBlock

	for _, part := range response.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			contentBlocks = append(contentBlocks, ResponsesContentBlock{
				Type: "output_text",
				Text: p.Text,
			})
		case cif.CIFThinkingPart:
			contentBlocks = append(contentBlocks, ResponsesContentBlock{
				Type: "output_text",
				Text: fmt.Sprintf("<thinking>\n%s\n</thinking>", p.Thinking),
			})
		case cif.CIFToolCallPart:
			args, _ := json.Marshal(p.ToolArguments)
			outputItems = append(outputItems, ResponsesOutputItem{
				Type:      "function_call",
				ID:        p.ToolCallID,
				Role:      "assistant",
				Name:      p.ToolName,
				Arguments: string(args),
			})
		}
	}

	if len(contentBlocks) > 0 {
		messageItem := ResponsesOutputItem{
			Type:    "message",
			ID:      fmt.Sprintf("%s-message", response.ID),
			Role:    "assistant",
			Content: contentBlocks,
		}
		outputItems = append([]ResponsesOutputItem{messageItem}, outputItems...)
	}

	resp := &ResponsesResponse{
		ID:        response.ID,
		Object:    "realtime.response",
		Model:     response.Model,
		Output:    outputItems,
		CreatedAt: time.Now().Unix(),
	}

	if response.Usage != nil {
		resp.Usage = &ResponsesUsage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
		}
	}

	return resp, nil
}

// Responses streaming state

type ResponsesStreamState struct {
	ResponseID       string
	Model            string
	CurrentItemID    string
	CurrentToolItemID string
	CurrentContentText string
	MessageItemAdded bool
}

func CreateResponsesStreamState() *ResponsesStreamState {
	return &ResponsesStreamState{}
}

// ConvertCIFEventToResponsesSSE converts CIF stream events to Responses API SSE events
func ConvertCIFEventToResponsesSSE(event cif.CIFStreamEvent, state *ResponsesStreamState) ([]map[string]interface{}, error) {
	var events []map[string]interface{}

	switch e := event.(type) {
	case cif.CIFStreamStart:
		state.ResponseID = e.ID
		state.Model = e.Model
		state.CurrentItemID = fmt.Sprintf("%s-message", e.ID)
		state.CurrentContentText = ""
		state.MessageItemAdded = false

		events = append(events, map[string]interface{}{
			"type": "response.created",
			"response": map[string]interface{}{
				"id":         e.ID,
				"object":     "realtime.response",
				"model":      e.Model,
				"output":     []interface{}{},
				"created_at": time.Now().Unix(),
			},
		})

	case cif.CIFContentDelta:
		if e.ContentBlock != nil {
			switch cb := e.ContentBlock.(type) {
			case cif.CIFTextPart, cif.CIFThinkingPart:
				_ = cb
				if !state.MessageItemAdded {
					events = append(events, map[string]interface{}{
						"type": "response.output_item.added",
						"item": map[string]interface{}{
							"type":    "message",
							"id":      state.CurrentItemID,
							"role":    "assistant",
							"content": []interface{}{},
						},
					})
					events = append(events, map[string]interface{}{
						"type": "response.content_block.added",
						"content_block": map[string]interface{}{
							"type": "output_text",
							"text": "",
						},
					})
					state.MessageItemAdded = true
				}
			case cif.CIFToolCallPart:
				toolItemID := fmt.Sprintf("%s-tool-%s", state.ResponseID, cb.ToolCallID)
				events = append(events, map[string]interface{}{
					"type": "response.output_item.added",
					"item": map[string]interface{}{
						"type":      "function_call",
						"id":        toolItemID,
						"role":      "assistant",
						"name":      cb.ToolName,
						"arguments": "",
					},
				})
				state.CurrentToolItemID = toolItemID
			}
		}

		switch d := e.Delta.(type) {
		case cif.TextDelta:
			state.CurrentContentText += d.Text
			events = append(events, map[string]interface{}{
				"type":  "response.output_text.delta",
				"delta": d.Text,
			})
		case cif.ThinkingDelta:
			state.CurrentContentText += d.Thinking
			events = append(events, map[string]interface{}{
				"type":  "response.output_text.delta",
				"delta": d.Thinking,
			})
		case cif.ToolArgumentsDelta:
			// Accumulated, sent in done event
		}

	case cif.CIFContentBlockStop:
		if state.MessageItemAdded && state.CurrentContentText != "" {
			events = append(events, map[string]interface{}{
				"type": "response.output_text.done",
				"text": state.CurrentContentText,
			})
			events = append(events, map[string]interface{}{
				"type": "response.output_item.done",
				"item": map[string]interface{}{
					"type": "message",
					"id":   state.CurrentItemID,
					"role": "assistant",
					"content": []map[string]interface{}{
						{"type": "output_text", "text": state.CurrentContentText},
					},
				},
			})
		}

	case cif.CIFStreamEnd:
		// Finalize remaining content
		if state.MessageItemAdded && state.CurrentContentText != "" {
			events = append(events, map[string]interface{}{
				"type": "response.output_text.done",
				"text": state.CurrentContentText,
			})
			events = append(events, map[string]interface{}{
				"type": "response.output_item.done",
				"item": map[string]interface{}{
					"type": "message",
					"id":   state.CurrentItemID,
					"role": "assistant",
					"content": []map[string]interface{}{
						{"type": "output_text", "text": state.CurrentContentText},
					},
				},
			})
		}

		completedResp := map[string]interface{}{
			"type": "response.completed",
			"response": map[string]interface{}{
				"id":         state.ResponseID,
				"object":     "realtime.response",
				"model":      state.Model,
				"output":     []interface{}{},
				"created_at": time.Now().Unix(),
			},
		}
		if e.Usage != nil {
			completedResp["response"].(map[string]interface{})["usage"] = map[string]interface{}{
				"input_tokens":  e.Usage.InputTokens,
				"output_tokens": e.Usage.OutputTokens,
			}
		}
		events = append(events, completedResp)
	}

	return events, nil
}
