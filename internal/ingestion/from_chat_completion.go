package ingestion

import (
	"encoding/json"
	"fmt"
	"omnillm/internal/cif"
	"strings"

	"github.com/rs/zerolog/log"
)

// OpenAI Chat Completions format
type OpenAIMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // string or array of content parts
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id,omitempty"`
	CallID   string       `json:"call_id,omitempty"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ContentPart struct {
	Type     string   `json:"type"`
	Text     string   `json:"text,omitempty"`
	ImageURL ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

type OpenAIToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type OpenAIChatCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  interface{}     `json:"tool_choice,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Stop        interface{}     `json:"stop,omitempty"`
	Stream      *bool           `json:"stream,omitempty"`
	User        string          `json:"user,omitempty"`
}

// ParseOpenAIChatCompletions converts OpenAI chat completions format to CIF
func ParseOpenAIChatCompletions(raw json.RawMessage) (*cif.CanonicalRequest, error) {
	var req OpenAIChatCompletionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OpenAI request: %w", err)
	}

	canonical := &cif.CanonicalRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream != nil && *req.Stream,
	}

	if req.User != "" {
		canonical.UserID = &req.User
	}

	if req.Stop != nil {
		switch stop := req.Stop.(type) {
		case string:
			canonical.Stop = []string{stop}
		case []any:
			for _, s := range stop {
				if str, ok := s.(string); ok {
					canonical.Stop = append(canonical.Stop, str)
				}
			}
		}
	}

	var systemParts []string
	var toolCallNamesByID = make(map[string]string)
	for _, msg := range req.Messages {
		if msg.Role == "system" || msg.Role == "developer" {
			if text := extractOpenAIMessageText(msg.Content); text != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}
		cifMsg, err := convertOpenAIMessage(msg, toolCallNamesByID)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		canonical.Messages = append(canonical.Messages, cifMsg)
	}
	if len(systemParts) > 0 {
		systemPrompt := strings.Join(systemParts, "\n\n")
		canonical.SystemPrompt = &systemPrompt
	}

	if len(req.Tools) > 0 {
		for _, tool := range req.Tools {
			var description *string
			if tool.Function.Description != "" {
				description = &tool.Function.Description
			}
			cifTool := cif.CIFTool{
				Name:             tool.Function.Name,
				Description:      description,
				ParametersSchema: tool.Function.Parameters,
			}
			canonical.Tools = append(canonical.Tools, cifTool)
		}
	}

	canonical.ToolChoice = normalizeOpenAIToolChoice(req.ToolChoice)

	log.Debug().
		Str("model", canonical.Model).
		Int("messages", len(canonical.Messages)).
		Int("tools", len(canonical.Tools)).
		Bool("stream", canonical.Stream).
		Msg("Converted OpenAI request to CIF")

	return canonical, nil
}

func convertOpenAIMessage(msg OpenAIMessage, toolCallNamesByID map[string]string) (cif.CIFMessage, error) {
	switch msg.Role {
	case "user":
		var contentParts []cif.CIFContentPart

		switch content := msg.Content.(type) {
		case string:
			contentParts = append(contentParts, cif.CIFTextPart{
				Type: "text",
				Text: content,
			})
		case []interface{}:
			for _, part := range content {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("invalid OpenAI content part type: %T", part)
				}
				cifPart, err := convertOpenAIContentPart(partMap)
				if err != nil {
					return nil, fmt.Errorf("failed to convert content part: %w", err)
				}
				contentParts = append(contentParts, cifPart)
			}
		}

		return cif.CIFUserMessage{
			Role:    "user",
			Content: contentParts,
		}, nil

	case "assistant":
		var contentParts []cif.CIFContentPart

		if str := extractOpenAIMessageText(msg.Content); str != "" {
			contentParts = append(contentParts, cif.CIFTextPart{
				Type: "text",
				Text: str,
			})
		}

		for _, toolCall := range msg.ToolCalls {
			args := parseOpenAIToolArguments(toolCall.Function.Arguments)
			toolCallID := firstNonEmpty(toolCall.ID, toolCall.CallID)
			if toolCallID != "" {
				toolCallNamesByID[toolCallID] = toolCall.Function.Name
			}

			contentParts = append(contentParts, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    toolCallID,
				ToolName:      toolCall.Function.Name,
				ToolArguments: args,
			})
		}

		return cif.CIFAssistantMessage{
			Role:    "assistant",
			Content: contentParts,
		}, nil

	case "tool":
		return cif.CIFUserMessage{
			Role: "user",
			Content: []cif.CIFContentPart{
				cif.CIFToolResultPart{
					Type:       "tool_result",
					ToolCallID: msg.ToolCallID,
					ToolName:   toolCallNamesByID[msg.ToolCallID],
					Content:    extractOpenAIMessageText(msg.Content),
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown message role: %s", msg.Role)
	}
}

func parseOpenAIToolArguments(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return map[string]interface{}{}
	}

	var args map[string]interface{}
	if err := json.Unmarshal(raw, &args); err == nil {
		return args
	}

	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		if err := json.Unmarshal([]byte(encoded), &args); err == nil {
			return args
		}
		return map[string]interface{}{
			"_unparsable_arguments": encoded,
		}
	}

	return map[string]interface{}{
		"_unparsable_arguments": string(raw),
	}
}

func convertOpenAIContentPart(partMap map[string]interface{}) (cif.CIFContentPart, error) {
	partType, ok := partMap["type"].(string)
	if !ok {
		return nil, fmt.Errorf("content part missing type")
	}

	switch partType {
	case "text", "input_text", "output_text":
		if text, ok := partMap["text"].(string); ok {
			return cif.CIFTextPart{
				Type: "text",
				Text: text,
			}, nil
		}
		return nil, fmt.Errorf("text content part missing text field")

	case "image_url", "input_image":
		url, err := extractOpenAIImageURL(partMap)
		if err != nil {
			return nil, err
		}

		part := cif.CIFImagePart{
			Type:      "image",
			MediaType: inferOpenAIImageMediaType(url),
		}

		if strings.HasPrefix(url, "data:") {
			if idx := strings.Index(url, ","); idx != -1 {
				data := url[idx+1:]
				part.Data = &data
			}
		} else {
			part.URL = &url
		}

		return part, nil

	default:
		return nil, fmt.Errorf("unknown content part type: %s", partType)
	}
}

func extractOpenAIImageURL(partMap map[string]interface{}) (string, error) {
	if imageURL, ok := partMap["image_url"].(map[string]interface{}); ok {
		if url, ok := imageURL["url"].(string); ok && url != "" {
			return url, nil
		}
	}
	if imageURL, ok := partMap["image_url"].(string); ok && imageURL != "" {
		return imageURL, nil
	}
	return "", fmt.Errorf("image content part missing url")
}

func inferOpenAIImageMediaType(url string) string {
	mediaType := "image/jpeg"
	if strings.Contains(url, "data:image/png") {
		mediaType = "image/png"
	} else if strings.Contains(url, "data:image/gif") {
		mediaType = "image/gif"
	} else if strings.Contains(url, "data:image/webp") {
		mediaType = "image/webp"
	}
	return mediaType
}


func extractOpenAIMessageText(content interface{}) string {
	switch value := content.(type) {
	case string:
		return value
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, rawPart := range value {
			partMap, ok := rawPart.(map[string]interface{})
			if !ok {
				continue
			}
			partType, _ := partMap["type"].(string)
			switch partType {
			case "text", "input_text", "output_text":
				if text, ok := partMap["text"].(string); ok {
					parts = append(parts, text)
				}
			default:
				if text, ok := partMap["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n\n")
	default:
		return ""
	}
}

func normalizeOpenAIToolChoice(choice interface{}) interface{} {
	switch value := choice.(type) {
	case string:
		switch value {
		case "none", "auto", "required":
			return value
		}
	case map[string]interface{}:
		if choiceType, _ := value["type"].(string); choiceType == "function" {
			if fn, ok := value["function"].(map[string]interface{}); ok {
				if name, ok := fn["name"].(string); ok && name != "" {
					return map[string]interface{}{
						"type":         "function",
						"functionName": name,
					}
				}
			}
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
