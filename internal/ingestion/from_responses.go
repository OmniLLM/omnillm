package ingestion

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"omnillm/internal/cif"

	"github.com/rs/zerolog/log"
)

// Responses API types

const (
	responsesTextMaxLength     = 10 * 1024 * 1024
	responsesImageURLMaxLength = 20 * 1024 * 1024
	responsesFileDataMaxLength = 70 * 1024 * 1024
)

type ResponsesPayload struct {
	Model                string          `json:"model"`
	Input                any             `json:"input"` // string or []InputItem
	Instructions         *string         `json:"instructions,omitempty"`
	Stream               *bool           `json:"stream,omitempty"`
	Temperature          *float64        `json:"temperature,omitempty"`
	TopP                 *float64        `json:"top_p,omitempty"`
	MaxOutputTokens      *int            `json:"max_output_tokens,omitempty"`
	Tools                []ResponsesTool `json:"tools,omitempty"`
	ToolChoice           any             `json:"tool_choice,omitempty"`
	PreviousResponseID   *string         `json:"previous_response_id,omitempty"`
	PromptCacheKey       *string         `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention *string         `json:"prompt_cache_retention,omitempty"`
	Store                *bool           `json:"store,omitempty"`
	Text                 *ResponsesText  `json:"text,omitempty"`
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
	Format      interface{}            `json:"format,omitempty"`
}

type InputItem struct {
	Type          string      `json:"type"`
	Role          string      `json:"role,omitempty"`
	Content       interface{} `json:"content,omitempty"` // string or []InputContentBlock
	ID            string      `json:"id,omitempty"`
	CallID        string      `json:"call_id,omitempty"`
	Name          string      `json:"name,omitempty"`
	Arguments     string      `json:"arguments,omitempty"`
	Input         interface{} `json:"input,omitempty"`
	InputPresent  bool        `json:"-"`
	Output        interface{} `json:"output,omitempty"`
	OutputPresent bool        `json:"-"`
	Namespace     string      `json:"namespace,omitempty"`
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
	if req.PromptCacheKey != nil || req.PromptCacheRetention != nil {
		canonical.PromptCache = &cif.CIFPromptCacheRequest{
			Key:       req.PromptCacheKey,
			Retention: req.PromptCacheRetention,
		}
	}

	if req.Text != nil && req.Text.Format != nil {
		canonical.ResponseFormat = translateResponsesTextFormat(req.Text.Format)
	}

	if req.Instructions != nil && *req.Instructions != "" {
		canonical.System = cif.SystemBlocksFromText(*req.Instructions)
	}

	messages, err := translateResponsesInput(req.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to translate input: %w", err)
	}
	canonical.Messages = messages

	canonical.Tools = translateResponsesTools(req.Tools)
	// Codex responses-lite (>=0.144) omits top-level "tools" and instead ships
	// an "additional_tools" input item carrying the tool definitions. Extract
	// those so translated backends still receive the tool set.
	if len(canonical.Tools) == 0 {
		canonical.Tools = extractAdditionalTools(req.Input)
	}
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
			inputItem := inputItemFromMap(itemMap)

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
						Content: cif.SystemBlocksFromText(extractInputText(inputItem.Content)),
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
				if inputItem.CallID == "" {
					return nil, fmt.Errorf("function_call_output item missing call_id")
				}
				if !inputItem.OutputPresent {
					return nil, fmt.Errorf("function_call_output item missing output")
				}
				output, rawOutput, err := normalizeToolOutput("function_call_output", inputItem.Output)
				if err != nil {
					return nil, err
				}
				flushAssistant()
				messages = append(messages, cif.CIFUserMessage{
					Role: "user",
					Content: []cif.CIFContentPart{
						cif.CIFToolResultPart{
							Type:       "tool_result",
							ToolCallID: inputItem.CallID,
							ToolName:   inputItem.Name,
							Content:    output,
							RawOutput:  rawOutput,
						},
					},
				})

			case "custom_tool_call":
				if inputItem.CallID == "" {
					return nil, fmt.Errorf("custom_tool_call item missing call_id")
				}
				if inputItem.Name == "" {
					return nil, fmt.Errorf("custom_tool_call item missing name")
				}
				if !inputItem.InputPresent {
					return nil, fmt.Errorf("custom_tool_call item missing input")
				}
				rawInput, ok := inputItem.Input.(string)
				if !ok {
					return nil, fmt.Errorf("custom_tool_call item input must be a string")
				}
				pendingAssistantParts = append(pendingAssistantParts, cif.CIFToolCallPart{
					Type:          "tool_call",
					ToolCallID:    inputItem.CallID,
					ToolName:      inputItem.Name,
					ToolArguments: map[string]interface{}{"input": rawInput},
					ToolKind:      cif.CIFToolKindCustom,
					RawInput:      &rawInput,
					Namespace:     inputItem.Namespace,
				})

			case "custom_tool_call_output":
				if inputItem.CallID == "" {
					return nil, fmt.Errorf("custom_tool_call_output item missing call_id")
				}
				if !inputItem.OutputPresent {
					return nil, fmt.Errorf("custom_tool_call_output item missing output")
				}
				output, _, err := normalizeToolOutput("custom_tool_call_output", inputItem.Output)
				if err != nil {
					return nil, err
				}
				flushAssistant()
				messages = append(messages, cif.CIFUserMessage{
					Role: "user",
					Content: []cif.CIFContentPart{
						cif.CIFToolResultPart{
							Type:         "tool_result",
							ToolCallID:   inputItem.CallID,
							ToolName:     inputItem.Name,
							Content:      output,
							ToolKind:     cif.CIFToolKindCustom,
							CustomOutput: inputItem.Output,
						},
					},
				})

			case "reasoning", "additional_tools":
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
		case item.OutputPresent && item.CallID != "":
			if _, ok := item.Output.(string); ok {
				itemType = "function_call_output"
			}
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
			cb := inputContentBlockFromMap(blockMap)
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
		var sb strings.Builder
		for _, block := range v {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if t, ok := blockMap["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
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

func normalizeToolOutput(itemType string, output interface{}) (string, interface{}, error) {
	switch value := output.(type) {
	case string:
		return value, nil, nil
	case []interface{}:
		for _, item := range value {
			content, ok := item.(map[string]interface{})
			if !ok {
				return "", nil, fmt.Errorf("%s item contains invalid content type: %T", itemType, item)
			}
			if err := validateToolOutputContent(itemType, content); err != nil {
				return "", nil, err
			}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", nil, fmt.Errorf("failed to encode %s content: %w", itemType, err)
		}
		return string(encoded), value, nil
	default:
		return "", nil, fmt.Errorf("%s item output must be a string or content list", itemType)
	}
}

func validateToolOutputContent(itemType string, content map[string]interface{}) error {
	contentType, _ := content["type"].(string)

	if itemType == "custom_tool_call_output" {
		switch contentType {
		case "input_text", "output_text", "text":
			if _, ok := content["text"].(string); !ok {
				return fmt.Errorf("%s %s content requires string field text", itemType, contentType)
			}
			return nil
		case "input_image":
			imageURL, hasImageURL := content["image_url"].(string)
			fileID, hasFileID := content["file_id"].(string)
			if hasImageURL && imageURL != "" || hasFileID && fileID != "" {
				return nil
			}
			return fmt.Errorf("%s input_image content requires image_url or file_id", itemType)
		case "input_file":
			fileID, hasFileID := content["file_id"].(string)
			fileURL, hasFileURL := content["file_url"].(string)
			filename, hasFilename := content["filename"].(string)
			_, hasFileData := content["file_data"].(string)
			if hasFileID && fileID != "" || hasFileURL && fileURL != "" || hasFilename && filename != "" && hasFileData {
				return nil
			}
			return fmt.Errorf("%s input_file content requires a supported file reference", itemType)
		default:
			return fmt.Errorf("%s item contains unknown content type: %s", itemType, contentType)
		}
	}

	requireString := func(field string, maxLength int) error {
		value, ok := content[field].(string)
		if !ok {
			return fmt.Errorf("%s %s content requires string field %s", itemType, contentType, field)
		}
		if utf8.RuneCountInString(value) > maxLength {
			return fmt.Errorf("%s %s content field %s exceeds maximum length", itemType, contentType, field)
		}
		return nil
	}
	optionalString := func(field string, maxLength int) error {
		value, present := content[field]
		if !present || value == nil {
			return nil
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s %s content field %s must be a string or null", itemType, contentType, field)
		}
		if maxLength > 0 && utf8.RuneCountInString(text) > maxLength {
			return fmt.Errorf("%s %s content field %s exceeds maximum length", itemType, contentType, field)
		}
		return nil
	}
	validateOptionalStrings := func(fields map[string]int) error {
		for field, maxLength := range fields {
			if err := optionalString(field, maxLength); err != nil {
				return err
			}
		}
		return nil
	}
	validateDetail := func(allowed ...string) error {
		value, present := content["detail"]
		if !present || value == nil {
			return nil
		}
		detail, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s %s content field detail must be a string or null", itemType, contentType)
		}
		for _, candidate := range allowed {
			if detail == candidate {
				return nil
			}
		}
		return fmt.Errorf("%s %s content has invalid detail: %s", itemType, contentType, detail)
	}

	switch contentType {
	case "input_text":
		return requireString("text", responsesTextMaxLength)
	case "input_image":
		if err := validateOptionalStrings(map[string]int{"image_url": responsesImageURLMaxLength, "file_id": 0}); err != nil {
			return err
		}
		return validateDetail("auto", "low", "high", "original")
	case "input_file":
		if err := validateOptionalStrings(map[string]int{"file_id": 0, "filename": 0, "file_data": responsesFileDataMaxLength, "file_url": 0}); err != nil {
			return err
		}
		return validateDetail("auto", "low", "high")
	default:
		return fmt.Errorf("%s item contains unknown content type: %s", itemType, contentType)
	}
}

// extractAdditionalTools pulls tool definitions out of a Codex responses-lite
// "additional_tools" input item. Codex nests tools either as flat function
// tools ({name, description, parameters}) or as namespaced groups
// ({type:"namespace", name, tools:[...]}). Nested tools keep their declared
// callable names; the namespace is transport grouping, not part of the name.
func extractAdditionalTools(input any) []cif.CIFTool {
	items, ok := input.([]interface{})
	if !ok {
		return nil
	}
	var cifTools []cif.CIFTool
	seenNames := map[string]struct{}{}
	appendTool := func(tm map[string]interface{}) {
		name, _ := tm["name"].(string)
		if name == "" {
			return
		}
		if _, seen := seenNames[name]; seen {
			return
		}
		seenNames[name] = struct{}{}
		desc, _ := tm["description"].(string)
		params, _ := tm["parameters"].(map[string]interface{})
		toolKind := cif.CIFToolKind("")
		var format interface{}
		if toolType, _ := tm["type"].(string); toolType == "custom" {
			toolKind = cif.CIFToolKindCustom
			format = tm["format"]
			params = customToolParametersSchema()
		}
		cifTools = append(cifTools, cif.CIFTool{
			Name:             name,
			Description:      &desc,
			ParametersSchema: params,
			ToolKind:         toolKind,
			Format:           format,
		})
	}
	for _, item := range items {
		im, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := im["type"].(string); t != "additional_tools" {
			continue
		}
		toolList, ok := im["tools"].([]interface{})
		if !ok {
			continue
		}
		for _, raw := range toolList {
			tm, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if tt, _ := tm["type"].(string); tt == "namespace" {
				nested, _ := tm["tools"].([]interface{})
				for _, nraw := range nested {
					if ntm, ok := nraw.(map[string]interface{}); ok {
						appendTool(ntm)
					}
				}
				continue
			}
			appendTool(tm)
		}
	}
	if len(cifTools) == 0 {
		return nil
	}
	return cifTools
}

func customToolParametersSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{"type": "string"},
		},
		"required":             []interface{}{"input"},
		"additionalProperties": false,
	}
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
		parameters := tool.Parameters
		toolKind := cif.CIFToolKind("")
		if tool.Type == "custom" {
			parameters = customToolParametersSchema()
			toolKind = cif.CIFToolKindCustom
		}
		cifTools = append(cifTools, cif.CIFTool{
			Name:             tool.Name,
			Description:      &desc,
			ParametersSchema: parameters,
			ToolKind:         toolKind,
			Format:           tool.Format,
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

// inputItemFromMap extracts an InputItem directly from a map[string]interface{}
// without a marshal+unmarshal roundtrip.
func inputItemFromMap(m map[string]interface{}) InputItem {
	getString := func(key string) string {
		v, _ := m[key].(string)
		return v
	}
	input, inputPresent := m["input"]
	output, outputPresent := m["output"]
	return InputItem{
		Type:          getString("type"),
		Role:          getString("role"),
		Content:       m["content"],
		ID:            getString("id"),
		CallID:        getString("call_id"),
		Name:          getString("name"),
		Arguments:     getString("arguments"),
		Input:         input,
		InputPresent:  inputPresent,
		Output:        output,
		OutputPresent: outputPresent,
		Namespace:     getString("namespace"),
	}
}

// inputContentBlockFromMap extracts an InputContentBlock directly from a
// map[string]interface{} without a marshal+unmarshal roundtrip.
func inputContentBlockFromMap(m map[string]interface{}) InputContentBlock {
	getString := func(key string) string {
		v, _ := m[key].(string)
		return v
	}
	return InputContentBlock{
		Type:     getString("type"),
		Text:     getString("text"),
		ImageURL: getString("image_url"),
		FileID:   getString("file_id"),
		Detail:   getString("detail"),
	}
}
