package shared

import "omnillm/internal/cif"

// CIFMessagesToGemini converts CIF messages to the Google Gemini contents format.
func CIFMessagesToGemini(messages []cif.CIFMessage) []map[string]interface{} {
	var contents []map[string]interface{}
	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			// System messages are handled via systemInstruction; skip here
			_ = m
		case cif.CIFUserMessage:
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"text": p.Text})
				case cif.CIFToolResultPart:
					parts = append(parts, map[string]interface{}{
						"functionResponse": map[string]interface{}{
							"name":     p.ToolName,
							"response": map[string]interface{}{"output": p.Content},
						},
					})
				case cif.CIFImagePart:
					if p.Data != nil {
						parts = append(parts, map[string]interface{}{
							"inlineData": map[string]interface{}{
								"mimeType": p.MediaType,
								"data":     *p.Data,
							},
						})
					}
				}
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]interface{}{"role": "user", "parts": parts})
			}
		case cif.CIFAssistantMessage:
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"text": p.Text})
				case cif.CIFToolCallPart:
					parts = append(parts, map[string]interface{}{
						"functionCall": map[string]interface{}{
							"name": p.ToolName,
							"args": p.ToolArguments,
						},
					})
				case cif.CIFThinkingPart:
					parts = append(parts, map[string]interface{}{"text": p.Thinking})
				}
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]interface{}{"role": "model", "parts": parts})
			}
		}
	}
	return contents
}

// SanitizeGeminiSchema removes fields that Gemini rejects from JSON Schema objects.
func SanitizeGeminiSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	blocked := map[string]bool{
		"$schema": true, "$id": true, "patternProperties": true, "prefill": true,
		"enumTitles": true, "deprecated": true, "propertyNames": true,
		"exclusiveMinimum": true, "exclusiveMaximum": true, "const": true,
	}
	clean := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		if blocked[k] {
			continue
		}
		switch nested := v.(type) {
		case map[string]interface{}:
			clean[k] = SanitizeGeminiSchema(nested)
		case []interface{}:
			cleaned := make([]interface{}, 0, len(nested))
			for _, item := range nested {
				if m, ok := item.(map[string]interface{}); ok {
					cleaned = append(cleaned, SanitizeGeminiSchema(m))
				} else {
					cleaned = append(cleaned, item)
				}
			}
			clean[k] = cleaned
		default:
			clean[k] = v
		}
	}
	return clean
}
