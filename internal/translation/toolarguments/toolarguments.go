// Package toolarguments normalizes completed tool-call arguments against the
// canonical tool schemas supplied by the caller.
package toolarguments

import (
	"context"
	"encoding/json"

	"omnillm/internal/cif"
)

// NormalizeMap removes top-level, declared, optional properties whose value is
// exactly the empty string. It returns the original map when no safe change can
// be made or no property changes.
func NormalizeMap(arguments map[string]interface{}, schema map[string]interface{}) map[string]interface{} {
	optional, ok := optionalProperties(schema)
	if !ok || len(optional) == 0 {
		return arguments
	}

	var normalized map[string]interface{}
	for name, value := range arguments {
		if value != "" || !optional[name] {
			continue
		}
		if normalized == nil {
			normalized = make(map[string]interface{}, len(arguments)-1)
			for key, original := range arguments {
				normalized[key] = original
			}
		}
		delete(normalized, name)
	}
	if normalized == nil {
		return arguments
	}
	return normalized
}

// NormalizeJSON applies NormalizeMap to a complete JSON object. Malformed JSON,
// non-object JSON, and uncertain schemas are returned byte-for-byte unchanged.
func NormalizeJSON(arguments string, schema map[string]interface{}) string {
	optional, ok := optionalProperties(schema)
	if !ok || len(optional) == 0 {
		return arguments
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &object); err != nil || object == nil {
		return arguments
	}
	changed := false
	for name, value := range object {
		if optional[name] && string(value) == `""` {
			delete(object, name)
			changed = true
		}
	}
	if !changed {
		return arguments
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return arguments
	}
	return string(encoded)
}

// NormalizeResponse returns response unchanged unless at least one tool call can
// be safely normalized. When a change is needed it copy-on-writes the response,
// content slice, tool part, and argument map.
func NormalizeResponse(response *cif.CanonicalResponse, tools []cif.CIFTool) *cif.CanonicalResponse {
	if response == nil || len(response.Content) == 0 || len(tools) == 0 {
		return response
	}

	schemas := schemasByName(tools)
	var content []cif.CIFContentPart
	for index, part := range response.Content {
		toolCall, ok := part.(cif.CIFToolCallPart)
		if !ok {
			continue
		}
		schema, ok := schemas[toolCall.ToolName]
		if !ok {
			continue
		}
		normalized := NormalizeMap(toolCall.ToolArguments, schema)
		if len(normalized) == len(toolCall.ToolArguments) {
			continue
		}
		if content == nil {
			content = append([]cif.CIFContentPart(nil), response.Content...)
		}
		toolCall.ToolArguments = normalized
		content[index] = toolCall
	}
	if content == nil {
		return response
	}
	copy := *response
	copy.Content = content
	return &copy
}

// NormalizeStream buffers only tool argument deltas. Announcements and all
// other content remain incremental. Each tool index is accumulated separately
// and emits one completed, normalized delta immediately before its stop, index
// reuse, terminal event, error, or upstream channel close.
func NormalizeStream(ctx context.Context, input <-chan cif.CIFStreamEvent, tools []cif.CIFTool) <-chan cif.CIFStreamEvent {
	output := make(chan cif.CIFStreamEvent)
	schemas := schemasByName(tools)
	normalizable := make(map[string]bool, len(schemas))
	for name, schema := range schemas {
		optional, ok := optionalProperties(schema)
		normalizable[name] = ok && len(optional) > 0
	}
	go func() {
		defer close(output)
		buffers := make(map[int]*streamBuffer)
		passThrough := make(map[int]bool)

		send := func(event cif.CIFStreamEvent) bool {
			select {
			case output <- event:
				return true
			case <-ctx.Done():
				return false
			}
		}
		flush := func(index int) bool {
			buffer := buffers[index]
			if buffer == nil {
				return true
			}
			delete(buffers, index)
			if !buffer.sawDelta {
				return true
			}
			arguments := buffer.arguments
			if schema, ok := schemas[buffer.toolName]; ok {
				arguments = NormalizeJSON(arguments, schema)
			}
			return send(cif.CIFContentDelta{
				Type:  "content_delta",
				Index: index,
				Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: arguments},
			})
		}
		flushAll := func() bool {
			// Preserve first-announcement order, not map iteration order.
			for {
				selectedIndex, selectedOrder := 0, int(^uint(0)>>1)
				found := false
				for index, buffer := range buffers {
					if buffer.order < selectedOrder {
						selectedIndex, selectedOrder, found = index, buffer.order, true
					}
				}
				if !found {
					return true
				}
				if !flush(selectedIndex) {
					return false
				}
			}
		}

		nextOrder := 0
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-input:
				if !ok {
					flushAll()
					return
				}

				switch current := event.(type) {
				case cif.CIFContentDelta:
					if current.ContentBlock != nil {
						announcement, isTool := current.ContentBlock.(cif.CIFToolCallPart)
						buffer := buffers[current.Index]
						if !isTool {
							delete(passThrough, current.Index)
							if !flush(current.Index) {
								return
							}
						} else if !normalizable[announcement.ToolName] {
							if !flush(current.Index) {
								return
							}
							passThrough[current.Index] = true
						} else {
							delete(passThrough, current.Index)
							if buffer != nil && buffer.toolName == "" && buffer.toolCallID == "" {
								buffer.toolName = announcement.ToolName
								buffer.toolCallID = announcement.ToolCallID
							} else if buffer != nil && (buffer.toolName != announcement.ToolName || buffer.toolCallID != announcement.ToolCallID) {
								if !flush(current.Index) {
									return
								}
								buffer = nil
							}
							if buffer == nil {
								buffer = &streamBuffer{toolName: announcement.ToolName, toolCallID: announcement.ToolCallID, order: nextOrder}
								nextOrder++
								buffers[current.Index] = buffer
							}
						}
					}
					if delta, ok := current.Delta.(cif.ToolArgumentsDelta); ok && !passThrough[current.Index] {
						buffer := buffers[current.Index]
						if buffer == nil {
							buffer = &streamBuffer{order: nextOrder}
							nextOrder++
							buffers[current.Index] = buffer
						}
						buffer.arguments += delta.PartialJSON
						buffer.sawDelta = true
						current.Delta = nil
					}
					if !send(current) {
						return
					}
				case cif.CIFContentBlockStop:
					delete(passThrough, current.Index)
					if !flush(current.Index) || !send(event) {
						return
					}
				case cif.CIFStreamEnd, cif.CIFStreamError:
					if !flushAll() || !send(event) {
						return
					}
				default:
					if !send(event) {
						return
					}
				}
			}
		}
	}()
	return output
}

type streamBuffer struct {
	toolName   string
	toolCallID string
	arguments  string
	order      int
	sawDelta   bool
}

func schemasByName(tools []cif.CIFTool) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{}, len(tools))
	for _, tool := range tools {
		if tool.Name != "" {
			result[tool.Name] = tool.ParametersSchema
		}
	}
	return result
}

func optionalProperties(schema map[string]interface{}) (map[string]bool, bool) {
	if schema == nil {
		return nil, false
	}
	if schemaType, ok := schema["type"]; !ok || schemaType != "object" {
		return nil, false
	}
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	required := make(map[string]bool)
	if raw, exists := schema["required"]; exists {
		items, ok := raw.([]interface{})
		if !ok {
			return nil, false
		}
		for _, item := range items {
			name, ok := item.(string)
			if !ok {
				return nil, false
			}
			required[name] = true
		}
	}
	optional := make(map[string]bool, len(properties))
	for name := range properties {
		if !required[name] {
			optional[name] = true
		}
	}
	return optional, true
}
