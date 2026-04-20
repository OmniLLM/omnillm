package serialization

import (
	"encoding/json"
	"fmt"
	"time"

	"omnillm/internal/cif"
)

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
	TotalTokens  int `json:"total_tokens"`
}

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
			TotalTokens:  response.Usage.InputTokens + response.Usage.OutputTokens,
		}
	}

	return resp, nil
}

type ResponsesStreamState struct {
	ResponseID        string
	Model             string
	CurrentItemID     string
	CurrentToolItemID string
	CurrentContentText string
	MessageItemAdded  bool
}

func CreateResponsesStreamState() *ResponsesStreamState {
	return &ResponsesStreamState{}
}

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
				"total_tokens":  e.Usage.InputTokens + e.Usage.OutputTokens,
			}
		}
		events = append(events, completedResp)
	}

	return events, nil
}
