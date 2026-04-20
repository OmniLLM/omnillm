package ingestion

import (
	"encoding/json"
	"fmt"

	"omnillm/internal/cif"

	"github.com/rs/zerolog/log"
)

// Responses API types

type ResponsesPayload struct {
	Model          string        `json:"model"`
	Input          interface{}   `json:"input"` // string or []InputItem
	Instructions   *string       `json:"instructions,omitempty"`
	Stream         *bool         `json:"stream,omitempty"`
	Temperature    *float64      `json:"temperature,omitempty"`
	TopP           *float64      `json:"top_p,omitempty"`
	MaxOutputTokens *int         `json:"max_output_tokens,omitempty"`
	Tools          []ResponsesTool `json:"tools,omitempty"`
	ToolChoice     interface{}   `json:"tool_choice,omitempty"`
}

type ResponsesTool struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type InputItem struct {
	Type      string      `json:"type"`
	Role      string      `json:"role,omitempty"`
	Content   interface{} `json:"content,omitempty"` // string or []InputContentBlock
	ID        string      `json:"id,omitempty"`
	CallID    string      `json:"call_id,omitempty"`
	Name      string      `json:"name,omitempty"`
	Arguments string      `json:"arguments,omitempty"`
	Output    string      `json:"output,omitempty"`
}

type InputContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ParseResponsesPayload converts Responses API payload to CIF
func ParseResponsesPayload(payload map[string]interface{}) (*cif.CanonicalRequest, error) {
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	var req ResponsesPayload
	if err := json.Unmarshal(jsonBytes, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Responses request: %w", err)
	}

	canonical := &cif.CanonicalRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxOutputTokens,
		Stream:      req.Stream != nil && *req.Stream,
	}

	// Set system prompt from instructions
	if req.Instructions != nil && *req.Instructions != "" {
		canonical.SystemPrompt = req.Instructions
	}

	// Convert input to messages
	messages, err := translateResponsesInput(req.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to translate input: %w", err)
	}
	canonical.Messages = messages

	// Convert tools
	canonical.Tools = translateResponsesTools(req.Tools)

	// Convert tool choice
	canonical.ToolChoice = translateResponsesToolChoice(req.ToolChoice)

	log.Debug().
		Str("model", canonical.Model).
		Int("messages", len(canonical.Messages)).
		Int("tools", len(canonical.Tools)).
		Bool("stream", canonical.Stream).
		Msg("Converted Responses request to CIF")

	return canonical, nil
}

func translateResponsesInput(input interface{}) ([]cif.CIFMessage, error) {
	switch v := input.(type) {
	case string:
		return []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: v}},
			},
		}, nil
	case []interface{}:
		var messages []cif.CIFMessage
		for _, item := range v {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			itemBytes, err := json.Marshal(itemMap)
			if err != nil {
				continue
			}
			var inputItem InputItem
			if err := json.Unmarshal(itemBytes, &inputItem); err != nil {
				continue
			}
			msgs, err := translateInputItem(inputItem, itemMap)
			if err != nil {
				return nil, err
			}
			messages = append(messages, msgs...)
		}
		return messages, nil
	default:
		return nil, fmt.Errorf("invalid input type")
	}
}

func translateInputItem(item InputItem, raw map[string]interface{}) ([]cif.CIFMessage, error) {
	switch item.Type {
	case "message":
		content, err := translateInputContent(item.Content)
		if err != nil {
			return nil, err
		}

		switch item.Role {
		case "system":
			text := extractInputText(item.Content)
			return []cif.CIFMessage{
				cif.CIFSystemMessage{Role: "system", Content: text},
			}, nil
		case "user":
			return []cif.CIFMessage{
				cif.CIFUserMessage{Role: "user", Content: content},
			}, nil
		case "assistant":
			return []cif.CIFMessage{
				cif.CIFAssistantMessage{Role: "assistant", Content: content},
			}, nil
		default:
			return nil, fmt.Errorf("unknown input item role: %s", item.Role)
		}

	case "function_call":
		toolCallID := item.CallID
		if toolCallID == "" {
			toolCallID = item.ID
		}
		if toolCallID == "" {
			return nil, fmt.Errorf("function_call item missing call_id and id")
		}

		args := parseToolArguments(item.Arguments)
		return []cif.CIFMessage{
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    toolCallID,
						ToolName:      item.Name,
						ToolArguments: args,
					},
				},
			},
		}, nil

	case "function_call_output":
		return []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFToolResultPart{
						Type:       "tool_result",
						ToolCallID: item.CallID,
						ToolName:   item.Name,
						Content:    item.Output,
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown input item type: %s", item.Type)
	}
}

func translateInputContent(content interface{}) ([]cif.CIFContentPart, error) {
	switch v := content.(type) {
	case string:
		return []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: v}}, nil
	case []interface{}:
		var parts []cif.CIFContentPart
		for _, block := range v {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			blockBytes, err := json.Marshal(blockMap)
			if err != nil {
				continue
			}
			var cb InputContentBlock
			if err := json.Unmarshal(blockBytes, &cb); err != nil {
				continue
			}
			switch cb.Type {
			case "input_text", "output_text":
				parts = append(parts, cif.CIFTextPart{Type: "text", Text: cb.Text})
			default:
				return nil, fmt.Errorf("unknown input content block type: %s", cb.Type)
			}
		}
		return parts, nil
	default:
		return []cif.CIFContentPart{}, nil
	}
}

func extractInputText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		text := ""
		for _, block := range v {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if t, ok := blockMap["text"].(string); ok {
					text += t
				}
			}
		}
		return text
	default:
		return ""
	}
}

func parseToolArguments(argumentsStr string) map[string]interface{} {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsStr), &parsed); err == nil {
		return parsed
	}
	return map[string]interface{}{"_unparsable_arguments": argumentsStr}
}

func translateResponsesTools(tools []ResponsesTool) []cif.CIFTool {
	if len(tools) == 0 {
		return nil
	}

	var cifTools []cif.CIFTool
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		desc := tool.Description
		cifTools = append(cifTools, cif.CIFTool{
			Name:             tool.Name,
			Description:      &desc,
			ParametersSchema: tool.Parameters,
		})
	}

	if len(cifTools) == 0 {
		return nil
	}
	return cifTools
}

func translateResponsesToolChoice(toolChoice interface{}) interface{} {
	if toolChoice == nil {
		return nil
	}

	switch v := toolChoice.(type) {
	case string:
		switch v {
		case "none", "auto", "required":
			return v
		default:
			return nil
		}
	case map[string]interface{}:
		if fn, ok := v["function"].(map[string]interface{}); ok {
			if name, ok := fn["name"].(string); ok {
				return map[string]interface{}{
					"type":         "function",
					"functionName": name,
				}
			}
		}
		return nil
	default:
		return nil
	}
}
