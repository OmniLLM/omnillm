package serialization

import (
	"encoding/json"
	"fmt"
	"omnillm/internal/cif"
	"strings"

	"github.com/rs/zerolog/log"
)

// Anthropic format structures
type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Model        string                  `json:"model"`
	Content      []AnthropicContentBlock `json:"content"`
	StopReason   *string                 `json:"stop_reason,omitempty"`
	StopSequence *string                 `json:"stop_sequence,omitempty"`
	Usage        *AnthropicUsage         `json:"usage,omitempty"`
}

type AnthropicContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Thinking  string                 `json:"thinking,omitempty"`
	Signature *string                `json:"signature,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func SerializeToAnthropic(response *cif.CanonicalResponse) (*AnthropicResponse, error) {
	return SerializeToAnthropicWithSuppression(response, false)
}

func SerializeToAnthropicWithSuppression(response *cif.CanonicalResponse, suppressThinking bool) (*AnthropicResponse, error) {
	var content []AnthropicContentBlock

	for _, part := range response.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			content = append(content, AnthropicContentBlock{
				Type: "text",
				Text: p.Text,
			})
		case cif.CIFToolCallPart:
			content = append(content, AnthropicContentBlock{
				Type:  "tool_use",
				ID:    normalizeAnthropicToolUseID(p.ToolCallID),
				Name:  p.ToolName,
				Input: p.ToolArguments,
			})
		case cif.CIFThinkingPart:
			if suppressThinking {
				continue
			}
			content = append(content, AnthropicContentBlock{
				Type:      "thinking",
				Thinking:  p.Thinking,
				Signature: p.Signature,
			})
		}
	}

	stopReason := convertStopReasonToAnthropic(response.StopReason)
	if !anthropicContentHasToolUse(content) && response.StopReason == cif.StopReasonToolUse {
		stopReason = convertStopReasonToAnthropic(cif.StopReasonEndTurn)
	}

	anthropicResp := &AnthropicResponse{
		ID:           response.ID,
		Type:         "message",
		Role:         "assistant",
		Model:        response.Model,
		Content:      content,
		StopReason:   stopReason,
		StopSequence: response.StopSequence,
	}

	if response.Usage != nil {
		anthropicResp.Usage = &AnthropicUsage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
		}
	}

	log.Debug().
		Str("id", response.ID).
		Str("model", response.Model).
		Str("stop_reason", *stopReason).
		Bool("suppress_thinking", suppressThinking).
		Msg("Converted CIF response to Anthropic format")

	return anthropicResp, nil
}

type AnthropicStreamState struct {
	MessageStartSent         bool
	NextContentBlockIndex    int
	ContentBlockOpen         bool
	CurrentBlockProviderIdx  int
	CurrentBlockAnthropicIdx int
	CurrentBlockType         string
	SuppressThinkingBlocks   bool
	SuppressedProviderIdx    int
}

func CreateAnthropicStreamState() *AnthropicStreamState {
	return &AnthropicStreamState{
		CurrentBlockProviderIdx:  -1,
		CurrentBlockAnthropicIdx: -1,
		SuppressedProviderIdx:    -1,
	}
}

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
				"id":    normalizeAnthropicToolUseID(cb.ToolCallID),
				"name":  cb.ToolName,
				"input": map[string]interface{}{},
			},
		}
	default:
		return nil
	}
}

func normalizeAnthropicToolUseID(id string) string {
	id = strings.TrimSpace(id)
	switch {
	case id == "":
		return "toolu_empty"
	case strings.HasPrefix(id, "toolu_"):
		return id
	case strings.HasPrefix(id, "tooluse_"):
		return "toolu_" + strings.TrimPrefix(id, "tooluse_")
	default:
		return "toolu_" + id
	}
}

func anthropicContentHasToolUse(content []AnthropicContentBlock) bool {
	for _, block := range content {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
}

func FormatAnthropicSSEData(eventType string, data interface{}) (string, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(jsonBytes)), nil
}

func convertStopReasonToAnthropic(reason cif.CIFStopReason) *string {
	var anthropicReason string
	switch reason {
	case cif.StopReasonEndTurn:
		anthropicReason = "end_turn"
	case cif.StopReasonMaxTokens:
		anthropicReason = "max_tokens"
	case cif.StopReasonToolUse:
		anthropicReason = "tool_use"
	case cif.StopReasonStopSequence:
		anthropicReason = "stop_sequence"
	case cif.StopReasonContentFilter:
		anthropicReason = "content_filter"
	case cif.StopReasonError:
		anthropicReason = "error"
	default:
		anthropicReason = "end_turn"
	}
	return &anthropicReason
}
