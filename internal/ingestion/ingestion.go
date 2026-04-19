// Package ingestion provides conversion from various API formats to Canonical Interface Format (CIF)
package ingestion

import (
	"encoding/json"
	"fmt"
	"strings"

	"omnimodel/internal/cif"

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
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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

// Anthropic Messages format
type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or array of content blocks
}

type AnthropicContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Thinking  string                 `json:"thinking,omitempty"`
	Signature string                 `json:"signature,omitempty"`
	Source    *AnthropicImageSource  `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	IsError   *bool                  `json:"is_error,omitempty"`
}

type AnthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type AnthropicMessagesRequest struct {
	Model       string             `json:"model"`
	System      interface{}        `json:"system,omitempty"`
	Messages    []AnthropicMessage `json:"messages"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice  interface{}        `json:"tool_choice,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	MaxTokens   *int               `json:"max_tokens,omitempty"`
	Stop        []string           `json:"stop_sequences,omitempty"`
	Stream      *bool              `json:"stream,omitempty"`
	Metadata    *struct {
		UserID string `json:"user_id,omitempty"`
	} `json:"metadata,omitempty"`
}

// ParseOpenAIChatCompletions converts OpenAI chat completions format to CIF
func ParseOpenAIChatCompletions(payload map[string]interface{}) (*cif.CanonicalRequest, error) {
	// Marshal back to JSON and unmarshal to our struct for type safety
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	var req OpenAIChatCompletionRequest
	if err := json.Unmarshal(jsonBytes, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OpenAI request: %w", err)
	}

	// Convert to CIF
	canonical := &cif.CanonicalRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream != nil && *req.Stream,
	}

	// Convert user ID
	if req.User != "" {
		canonical.UserID = &req.User
	}

	// Convert stop sequences
	if req.Stop != nil {
		switch stop := req.Stop.(type) {
		case string:
			canonical.Stop = []string{stop}
		case []string:
			canonical.Stop = stop
		case []interface{}:
			for _, s := range stop {
				if str, ok := s.(string); ok {
					canonical.Stop = append(canonical.Stop, str)
				}
			}
		}
	}

	var systemParts []string
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if text := extractOpenAIMessageText(msg.Content); text != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}
		cifMsg, err := convertOpenAIMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		canonical.Messages = append(canonical.Messages, cifMsg)
	}
	if len(systemParts) > 0 {
		systemPrompt := strings.Join(systemParts, "\n\n")
		canonical.SystemPrompt = &systemPrompt
	}

	// Convert tools
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

	// Convert tool choice
	canonical.ToolChoice = normalizeOpenAIToolChoice(req.ToolChoice)

	log.Debug().
		Str("model", canonical.Model).
		Int("messages", len(canonical.Messages)).
		Int("tools", len(canonical.Tools)).
		Bool("stream", canonical.Stream).
		Msg("Converted OpenAI request to CIF")

	return canonical, nil
}

func convertOpenAIMessage(msg OpenAIMessage) (cif.CIFMessage, error) {
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
				if partMap, ok := part.(map[string]interface{}); ok {
					cifPart, err := convertOpenAIContentPart(partMap)
					if err != nil {
						return nil, fmt.Errorf("failed to convert content part: %w", err)
					}
					contentParts = append(contentParts, cifPart)
				}
			}
		}

		return cif.CIFUserMessage{
			Role:    "user",
			Content: contentParts,
		}, nil

	case "assistant":
		var contentParts []cif.CIFContentPart

		// Handle text content
		if str := extractOpenAIMessageText(msg.Content); str != "" {
			contentParts = append(contentParts, cif.CIFTextPart{
				Type: "text",
				Text: str,
			})
		}

		// Handle tool calls
		for _, toolCall := range msg.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				args = map[string]interface{}{
					"_unparsable_arguments": toolCall.Function.Arguments,
				}
			}

			contentParts = append(contentParts, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    toolCall.ID,
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
					Content:    extractOpenAIMessageText(msg.Content),
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown message role: %s", msg.Role)
	}
}

func convertOpenAIContentPart(partMap map[string]interface{}) (cif.CIFContentPart, error) {
	partType, ok := partMap["type"].(string)
	if !ok {
		return nil, fmt.Errorf("content part missing type")
	}

	switch partType {
	case "text":
		if text, ok := partMap["text"].(string); ok {
			return cif.CIFTextPart{
				Type: "text",
				Text: text,
			}, nil
		}
		return nil, fmt.Errorf("text content part missing text field")

	case "image_url":
		if imageURL, ok := partMap["image_url"].(map[string]interface{}); ok {
			if url, ok := imageURL["url"].(string); ok {
				mediaType := "image/jpeg" // Default
				if strings.Contains(url, "data:image/png") {
					mediaType = "image/png"
				} else if strings.Contains(url, "data:image/gif") {
					mediaType = "image/gif"
				} else if strings.Contains(url, "data:image/webp") {
					mediaType = "image/webp"
				}

				part := cif.CIFImagePart{
					Type:      "image",
					MediaType: mediaType,
				}

				if strings.HasPrefix(url, "data:") {
					// Extract base64 data
					if idx := strings.Index(url, ","); idx != -1 {
						data := url[idx+1:]
						part.Data = &data
					}
				} else {
					part.URL = &url
				}

				return part, nil
			}
		}
		return nil, fmt.Errorf("image_url content part missing url")

	default:
		return nil, fmt.Errorf("unknown content part type: %s", partType)
	}
}

// ParseAnthropicMessages converts Anthropic messages format to CIF
func ParseAnthropicMessages(payload map[string]interface{}) (*cif.CanonicalRequest, error) {
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	var req AnthropicMessagesRequest
	if err := json.Unmarshal(jsonBytes, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Anthropic request: %w", err)
	}

	canonical := &cif.CanonicalRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stop:        req.Stop,
		Stream:      req.Stream != nil && *req.Stream,
	}

	// Set system prompt if present
	if systemPrompt := normalizeAnthropicSystem(req.System); systemPrompt != nil {
		canonical.SystemPrompt = systemPrompt
	}
	if req.Metadata != nil && req.Metadata.UserID != "" {
		canonical.UserID = &req.Metadata.UserID
	}

	// Convert messages
	for _, msg := range req.Messages {
		cifMsg, err := convertAnthropicMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		canonical.Messages = append(canonical.Messages, cifMsg)
	}

	// Convert tools
	for _, tool := range req.Tools {
		var description *string
		if tool.Description != "" {
			description = &tool.Description
		}
		cifTool := cif.CIFTool{
			Name:             tool.Name,
			Description:      description,
			ParametersSchema: normalizeSchema(tool.InputSchema),
		}
		canonical.Tools = append(canonical.Tools, cifTool)
	}

	canonical.ToolChoice = normalizeAnthropicToolChoice(req.ToolChoice)

	log.Debug().
		Str("model", canonical.Model).
		Int("messages", len(canonical.Messages)).
		Int("tools", len(canonical.Tools)).
		Bool("stream", canonical.Stream).
		Msg("Converted Anthropic request to CIF")

	return canonical, nil
}

func convertAnthropicMessage(msg AnthropicMessage) (cif.CIFMessage, error) {
	contentParts, err := convertAnthropicMessageContent(msg.Content)
	if err != nil {
		return nil, err
	}

	switch msg.Role {
	case "user":
		return cif.CIFUserMessage{
			Role:    "user",
			Content: contentParts,
		}, nil

	case "assistant":
		return cif.CIFAssistantMessage{
			Role:    "assistant",
			Content: contentParts,
		}, nil

	default:
		return nil, fmt.Errorf("unknown message role: %s", msg.Role)
	}
}

func convertAnthropicMessageContent(content interface{}) ([]cif.CIFContentPart, error) {
	switch raw := content.(type) {
	case string:
		return []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: raw},
		}, nil
	case []interface{}:
		contentParts := make([]cif.CIFContentPart, 0, len(raw))
		for _, blockRaw := range raw {
			blockMap, ok := blockRaw.(map[string]interface{})
			if !ok {
				continue
			}
			blockBytes, err := json.Marshal(blockMap)
			if err != nil {
				return nil, err
			}
			var block AnthropicContentBlock
			if err := json.Unmarshal(blockBytes, &block); err != nil {
				return nil, err
			}
			cifPart, err := convertAnthropicContentBlock(block)
			if err != nil {
				return nil, err
			}
			contentParts = append(contentParts, cifPart)
		}
		return contentParts, nil
	case nil:
		return []cif.CIFContentPart{}, nil
	default:
		return nil, fmt.Errorf("unsupported Anthropic message content type: %T", content)
	}
}

func convertAnthropicContentBlock(block AnthropicContentBlock) (cif.CIFContentPart, error) {
	switch block.Type {
	case "text":
		return cif.CIFTextPart{
			Type: "text",
			Text: block.Text,
		}, nil

	case "image":
		if block.Source != nil {
			part := cif.CIFImagePart{
				Type:      "image",
				MediaType: block.Source.MediaType,
			}
			if block.Source.Type == "base64" {
				part.Data = &block.Source.Data
			}
			return part, nil
		}
		return nil, fmt.Errorf("image block missing source")

	case "thinking":
		part := cif.CIFThinkingPart{
			Type:     "thinking",
			Thinking: block.Thinking,
		}
		if block.Signature != "" {
			signature := block.Signature
			part.Signature = &signature
		}
		return part, nil

	case "tool_use":
		toolCallID := block.ToolUseID
		if toolCallID == "" {
			toolCallID = block.ID
		}
		return cif.CIFToolCallPart{
			Type:          "tool_call",
			ToolCallID:    toolCallID,
			ToolName:      block.Name,
			ToolArguments: block.Input,
		}, nil

	case "tool_result":
		toolCallID := block.ToolUseID
		if toolCallID == "" {
			toolCallID = block.ID
		}
		return cif.CIFToolResultPart{
			Type:       "tool_result",
			ToolCallID: toolCallID,
			ToolName:   block.Name,
			Content:    normalizeAnthropicToolResultContent(block.Content),
			IsError:    block.IsError,
		}, nil

	default:
		return nil, fmt.Errorf("unknown content block type: %s", block.Type)
	}
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
			if text, ok := partMap["text"].(string); ok {
				parts = append(parts, text)
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

func normalizeAnthropicSystem(system interface{}) *string {
	switch value := system.(type) {
	case string:
		if value == "" {
			return nil
		}
		return &value
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, rawPart := range value {
			partMap, ok := rawPart.(map[string]interface{})
			if !ok {
				continue
			}
			if partType, _ := partMap["type"].(string); partType != "text" {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			return nil
		}
		result := strings.Join(parts, "\n\n")
		return &result
	default:
		return nil
	}
}

func normalizeAnthropicToolChoice(choice interface{}) interface{} {
	choiceMap, ok := choice.(map[string]interface{})
	if !ok {
		return nil
	}

	switch choiceType, _ := choiceMap["type"].(string); choiceType {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		if name, ok := choiceMap["name"].(string); ok && name != "" {
			return map[string]interface{}{
				"type":         "function",
				"functionName": name,
			}
		}
	}

	return nil
}

func normalizeAnthropicToolResultContent(content interface{}) string {
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
			if text, ok := partMap["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	case nil:
		return ""
	}

	if jsonBytes, err := json.Marshal(content); err == nil {
		return string(jsonBytes)
	}
	return ""
}

func normalizeSchema(schema interface{}) map[string]interface{} {
	normalized, ok := normalizeSchemaValue(schema).(map[string]interface{})
	if !ok {
		return nil
	}
	return normalized
}

func normalizeSchemaValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if key == "$schema" || key == "patternProperties" {
				continue
			}
			result[key] = normalizeSchemaValue(item)
		}

		if nullable, ok := result["nullable"].(bool); ok && nullable {
			delete(result, "nullable")
			switch currentType := result["type"].(type) {
			case string:
				result["type"] = []interface{}{currentType, "null"}
			case []interface{}:
				hasNull := false
				for _, item := range currentType {
					if text, ok := item.(string); ok && text == "null" {
						hasNull = true
						break
					}
				}
				if !hasNull {
					result["type"] = append(currentType, "null")
				}
			}
		}

		return result
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, normalizeSchemaValue(item))
		}
		return result
	default:
		return value
	}
}
