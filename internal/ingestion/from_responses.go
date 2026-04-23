package ingestion

import (
	"encoding/json"
	"fmt"
	"omnillm/internal/cif"

	"github.com/rs/zerolog/log"
)

// Responses API types

type ResponsesPayload struct {
	Model              string          `json:"model"`
	Input              any             `json:"input"` // string or []InputItem
	Instructions       *string         `json:"instructions,omitempty"`
	Stream             *bool           `json:"stream,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	TopP               *float64        `json:"top_p,omitempty"`
	MaxOutputTokens    *int            `json:"max_output_tokens,omitempty"`
	Tools              []ResponsesTool `json:"tools,omitempty"`
	ToolChoice         any             `json:"tool_choice,omitempty"`
	PreviousResponseID *string         `json:"previous_response_id,omitempty"`
	Store              *bool           `json:"store,omitempty"`
	Text               *ResponsesText  `json:"text,omitempty"`
}

// ResponsesText holds the text.format structured output configuration.
type ResponsesText struct {
	Format *ResponsesTextFormat `json:"format,omitempty"`
}

// ResponsesTextFormat mirrors the response_format shape but nested under text.format.
type ResponsesTextFormat struct {
	Type       string                 `json:"type"`
	Name       string                 `json:"name,omitempty"`
	Strict     *bool                  `json:"strict,omitempty"`
	Schema     map[string]interface{} `json:"schema,omitempty"`
	JSONSchema map[string]interface{} `json:"json_schema,omitempty"`
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
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// ParseResponsesPayload converts Responses API payload to CIF
func ParseResponsesPayload(raw json.RawMessage) (*cif.CanonicalRequest, error) {
	var req ResponsesPayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Responses request: %w", err)
	}

	canonical := &cif.CanonicalRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxOutputTokens,
		Stream:      req.Stream != nil && *req.Stream,
	}

	if req.PreviousResponseID != nil && *req.PreviousResponseID != "" {
		canonical.PreviousResponseID = req.PreviousResponseID
	}

	if req.Text != nil && req.Text.Format != nil {
		canonical.ResponseFormat = translateResponsesTextFormat(req.Text.Format)
	}

	if req.Instructions != nil && *req.Instructions != "" {
		canonical.SystemPrompt = req.Instructions
	}

	messages, err := translateResponsesInput(req.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to translate input: %w", err)
	}
	canonical.Messages = messages

	canonical.Tools = translateResponsesTools(req.Tools)
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
		var pendingAssistantParts []cif.CIFContentPart

		flushAssistant := func() {
			if len(pendingAssistantParts) == 0 {
				return
			}
			content := append([]cif.CIFContentPart(nil), pendingAssistantParts...)
			messages = append(messages, cif.CIFAssistantMessage{
				Role:    "assistant",
				Content: content,
			})
			pendingAssistantParts = nil
		}

		for _, item := range v {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid input item type: %T", item)
			}
			itemBytes, err := json.Marshal(itemMap)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal input item: %w", err)
			}
			var inputItem InputItem
			if err := json.Unmarshal(itemBytes, &inputItem); err != nil {
				return nil, fmt.Errorf("failed to decode input item: %w", err)
			}

			switch inputItemType(inputItem) {
			case "message":
				content, err := translateInputContent(inputItem.Content)
				if err != nil {
					return nil, err
				}

				switch inputItem.Role {
				case "system", "developer":
					flushAssistant()
					messages = append(messages, cif.CIFSystemMessage{
						Role:    "system",
						Content: extractInputText(inputItem.Content),
					})
				case "user":
					flushAssistant()
					messages = append(messages, cif.CIFUserMessage{
						Role:    "user",
						Content: content,
					})
				case "assistant":
					pendingAssistantParts = append(pendingAssistantParts, content...)
				default:
					return nil, fmt.Errorf("unknown input item role: %s", inputItem.Role)
				}

			case "function_call":
				toolCallID := inputItem.CallID
				if toolCallID == "" {
					toolCallID = inputItem.ID
				}
				if toolCallID == "" {
					return nil, fmt.Errorf("function_call item missing call_id and id")
				}

				pendingAssistantParts = append(pendingAssistantParts, cif.CIFToolCallPart{
					Type:          "tool_call",
					ToolCallID:    toolCallID,
					ToolName:      inputItem.Name,
					ToolArguments: parseToolArguments(inputItem.Arguments),
				})

			case "function_call_output":
				flushAssistant()
				messages = append(messages, cif.CIFUserMessage{
					Role: "user",
					Content: []cif.CIFContentPart{
						cif.CIFToolResultPart{
							Type:       "tool_result",
							ToolCallID: inputItem.CallID,
							ToolName:   inputItem.Name,
							Content:    inputItem.Output,
						},
					},
				})

			case "reasoning":
				continue

			default:
				return nil, fmt.Errorf("unknown input item type: %s", inputItem.Type)
			}
		}
		flushAssistant()
		return messages, nil
	default:
		return nil, fmt.Errorf("invalid input type")
	}
}

func inputItemType(item InputItem) string {
	itemType := item.Type
	if itemType == "" {
		switch {
		case item.Role != "" && item.Content != nil:
			itemType = "message"
		case item.Output != "" && item.CallID != "":
			itemType = "function_call_output"
		case item.Name != "" && (item.Arguments != "" || item.ID != "" || item.CallID != ""):
			itemType = "function_call"
		}
	}
	return itemType
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
				return nil, fmt.Errorf("invalid input content block type: %T", block)
			}
			blockBytes, err := json.Marshal(blockMap)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal input content block: %w", err)
			}
			var cb InputContentBlock
			if err := json.Unmarshal(blockBytes, &cb); err != nil {
				return nil, fmt.Errorf("failed to decode input content block: %w", err)
			}
			switch cb.Type {
			case "input_text", "output_text", "text":
				parts = append(parts, cif.CIFTextPart{Type: "text", Text: cb.Text})
			case "input_image":
				part := cif.CIFImagePart{Type: "image"}
				if cb.ImageURL != "" {
					part.URL = &cb.ImageURL
				}
				parts = append(parts, part)
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

// translateResponsesTextFormat converts the Responses API text.format object
// into the canonical response_format map used by outbound adapters.
func translateResponsesTextFormat(f *ResponsesTextFormat) map[string]interface{} {
	if f == nil {
		return nil
	}
	switch f.Type {
	case "json_schema":
		strict := true
		if f.Strict != nil {
			strict = *f.Strict
		}
		schema := f.Schema
		if schema == nil {
			schema = f.JSONSchema
		}
		jsonSchemaObj := map[string]interface{}{
			"name":   f.Name,
			"strict": strict,
			"schema": schema,
		}
		return map[string]interface{}{
			"type":        "json_schema",
			"json_schema": jsonSchemaObj,
		}
	case "json_object":
		return map[string]interface{}{"type": "json_object"}
	case "text", "":
		return map[string]interface{}{"type": "text"}
	default:
		return map[string]interface{}{"type": f.Type}
	}
}
