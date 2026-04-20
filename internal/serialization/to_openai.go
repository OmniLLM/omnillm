package serialization

import (
	"encoding/json"
	"fmt"
	"time"

	"omnillm/internal/cif"

	"github.com/rs/zerolog/log"
)

// OpenAI format structures
type OpenAIResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	Choices           []OpenAIChoice `json:"choices"`
	Usage             *OpenAIUsage   `json:"usage,omitempty"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
}

type OpenAIChoice struct {
	Index        int            `json:"index"`
	Message      *OpenAIMessage `json:"message,omitempty"`
	Delta        *OpenAIDelta   `json:"delta,omitempty"`
	FinishReason *string        `json:"finish_reason"`
	LogProbs     interface{}    `json:"logprobs,omitempty"`
}

type OpenAIMessage struct {
	Role      string           `json:"role"`
	Content   *string          `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
	Name      string           `json:"name,omitempty"`
}

type OpenAIDelta struct {
	Role      string                `json:"role,omitempty"`
	Content   *string               `json:"content,omitempty"`
	ToolCalls []OpenAIToolCallDelta `json:"tool_calls,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIToolCallDelta struct {
	Index    int                 `json:"index"`
	ID       string              `json:"id,omitempty"`
	Type     string              `json:"type,omitempty"`
	Function *OpenAIFunctionCall `json:"function,omitempty"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SerializeToOpenAI converts a CIF response to OpenAI format
func SerializeToOpenAI(response *cif.CanonicalResponse) (*OpenAIResponse, error) {
	var contentText string
	var toolCalls []OpenAIToolCall

	for _, part := range response.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			contentText += p.Text
		case cif.CIFToolCallPart:
			args, err := json.Marshal(p.ToolArguments)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool arguments: %w", err)
			}

			toolCall := OpenAIToolCall{
				ID:   p.ToolCallID,
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      p.ToolName,
					Arguments: string(args),
				},
			}
			toolCalls = append(toolCalls, toolCall)
		}
	}

	finishReason := convertStopReasonToOpenAI(response.StopReason)

	choice := OpenAIChoice{
		Index: 0,
		Message: &OpenAIMessage{
			Role: "assistant",
		},
		FinishReason: finishReason,
	}

	if contentText != "" {
		choice.Message.Content = &contentText
	}

	if len(toolCalls) > 0 {
		choice.Message.ToolCalls = toolCalls
	}

	openaiResp := &OpenAIResponse{
		ID:      response.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   response.Model,
		Choices: []OpenAIChoice{choice},
	}

	if response.Usage != nil {
		openaiResp.Usage = &OpenAIUsage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		}
	}

	log.Debug().
		Str("id", response.ID).
		Str("model", response.Model).
		Str("finish_reason", *finishReason).
		Msg("Converted CIF response to OpenAI format")

	return openaiResp, nil
}

type OpenAIStreamChunk struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	Choices           []OpenAIChoice `json:"choices"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
}

type OpenAIStreamState struct {
	ID            string
	Model         string
	Created       int64
	Index         int
	ToolCallIndex int
}

func CreateOpenAIStreamState() *OpenAIStreamState {
	return &OpenAIStreamState{
		ID:            fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Model:         "",
		Created:       time.Now().Unix(),
		Index:         0,
		ToolCallIndex: 0,
	}
}

func ConvertCIFEventToOpenAISSE(event cif.CIFStreamEvent, state *OpenAIStreamState) (string, error) {
	switch e := event.(type) {
	case cif.CIFStreamStart:
		state.Model = e.Model
		state.ID = e.ID
		state.ToolCallIndex = 0
		chunk := OpenAIStreamChunk{
			ID:      e.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   e.Model,
			Choices: []OpenAIChoice{{
				Index: 0,
				Delta: &OpenAIDelta{Role: "assistant"},
			}},
		}
		return formatSSEData(chunk)

	case cif.CIFContentDelta:
		if e.ContentBlock != nil {
			switch cb := e.ContentBlock.(type) {
			case cif.CIFToolCallPart:
				toolCallDelta := OpenAIDelta{
					ToolCalls: []OpenAIToolCallDelta{{
						Index: state.ToolCallIndex,
						ID:    cb.ToolCallID,
						Type:  "function",
						Function: &OpenAIFunctionCall{
							Name:      cb.ToolName,
							Arguments: "",
						},
					}},
				}
				state.ToolCallIndex++
				chunk := OpenAIStreamChunk{
					ID:      state.ID,
					Object:  "chat.completion.chunk",
					Created: state.Created,
					Model:   state.Model,
					Choices: []OpenAIChoice{{
						Index: 0,
						Delta: &toolCallDelta,
					}},
				}
				return formatSSEData(chunk)
			}
		}

		var delta OpenAIDelta

		switch d := e.Delta.(type) {
		case cif.TextDelta:
			delta.Content = &d.Text
		case cif.ThinkingDelta:
			delta.Content = &d.Thinking
		case cif.ToolArgumentsDelta:
			delta.ToolCalls = []OpenAIToolCallDelta{{
				Index: state.ToolCallIndex - 1,
				Function: &OpenAIFunctionCall{
					Arguments: d.PartialJSON,
				},
			}}
		default:
			return "", nil
		}

		chunk := OpenAIStreamChunk{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []OpenAIChoice{{
				Index: 0,
				Delta: &delta,
			}},
		}
		return formatSSEData(chunk)

	case cif.CIFContentBlockStop:
		return "", nil

	case cif.CIFStreamEnd:
		finishReason := convertStopReasonToOpenAI(e.StopReason)
		chunk := OpenAIStreamChunk{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []OpenAIChoice{{
				Index:        0,
				FinishReason: finishReason,
				Delta:        &OpenAIDelta{},
			}},
		}

		sseData, err := formatSSEData(chunk)
		if err != nil {
			return "", err
		}

		if e.Usage != nil {
			usageChunk := map[string]interface{}{
				"id":      state.ID,
				"object":  "chat.completion.chunk",
				"created": state.Created,
				"model":   state.Model,
				"choices": []interface{}{},
				"usage": map[string]interface{}{
					"prompt_tokens":     e.Usage.InputTokens,
					"completion_tokens": e.Usage.OutputTokens,
					"total_tokens":      e.Usage.InputTokens + e.Usage.OutputTokens,
				},
			}
			usageSSE, err := formatSSEData(usageChunk)
			if err != nil {
				return sseData, nil
			}
			sseData += usageSSE
		}

		sseData += "data: [DONE]\n\n"
		return sseData, nil

	case cif.CIFStreamError:
		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": e.Error.Message,
				"type":    e.Error.Type,
			},
		}
		return formatSSEData(errorChunk)

	default:
		return "", nil
	}
}

func formatSSEData(data interface{}) (string, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("data: %s\n\n", string(jsonBytes)), nil
}

func convertStopReasonToOpenAI(reason cif.CIFStopReason) *string {
	var openaiReason string
	switch reason {
	case cif.StopReasonEndTurn:
		openaiReason = "stop"
	case cif.StopReasonMaxTokens:
		openaiReason = "length"
	case cif.StopReasonToolUse:
		openaiReason = "tool_calls"
	case cif.StopReasonStopSequence:
		openaiReason = "stop"
	case cif.StopReasonContentFilter:
		openaiReason = "content_filter"
	case cif.StopReasonError:
		openaiReason = "stop"
	default:
		openaiReason = "stop"
	}
	return &openaiReason
}
