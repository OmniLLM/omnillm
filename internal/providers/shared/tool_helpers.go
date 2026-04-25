package shared

import (
	"encoding/json"
	"strings"
)

// NormalizeToolArguments converts arbitrary raw tool args to map[string]interface{}.
func NormalizeToolArguments(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case nil:
		return map[string]interface{}{}
	case map[string]interface{}:
		if value == nil {
			return map[string]interface{}{}
		}
		return value
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return map[string]interface{}{}
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && parsed != nil {
			return parsed
		}
		return map[string]interface{}{"value": value}
	case []interface{}:
		return map[string]interface{}{"items": value}
	default:
		return map[string]interface{}{"value": value}
	}
}

// ConvertCanonicalToolChoiceToOpenAI converts a CIF tool choice to OpenAI format.
func ConvertCanonicalToolChoiceToOpenAI(toolChoice interface{}) interface{} {
	switch choice := toolChoice.(type) {
	case string:
		switch choice {
		case "none", "auto", "required":
			return choice
		default:
			return nil
		}
	case map[string]interface{}:
		functionName, _ := choice["functionName"].(string)
		if functionName == "" {
			if function, ok := choice["function"].(map[string]interface{}); ok {
				functionName, _ = function["name"].(string)
			}
		}
		if functionName == "" {
			return nil
		}
		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": functionName,
			},
		}
	default:
		return nil
	}
}

// NormalizeToolParameters ensures tool parameters are never nil.
// The OpenAI-compatible API (used by Qwen/DashScope, Azure, etc.) expects
// "parameters" to be a JSON Schema object, defaulting to {}. Serialising nil
// produces "parameters": null which some providers reject.
func NormalizeToolParameters(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}
	return schema
}
