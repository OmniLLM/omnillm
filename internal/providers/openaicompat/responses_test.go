package openaicompat

import (
	"reflect"
	"testing"

	"omnillm/internal/cif"
)

func TestResponsesUserMessageItemsPreservesStructuredFunctionOutput(t *testing.T) {
	output := []interface{}{
		map[string]interface{}{"type": "input_text", "text": "ok"},
		map[string]interface{}{"type": "input_file", "file_id": "file_123"},
	}
	items := responsesUserMessageItems(cif.CIFUserMessage{
		Role: "user",
		Content: []cif.CIFContentPart{
			cif.CIFToolResultPart{
				Type:       "tool_result",
				ToolCallID: "call_1",
				Content:    `[{"type":"input_text","text":"ok"},{"type":"input_file","file_id":"file_123"}]`,
				RawOutput:  output,
			},
		},
	}, false)

	if len(items) != 1 || items[0]["type"] != "function_call_output" {
		t.Fatalf("Responses items = %#v", items)
	}
	if !reflect.DeepEqual(items[0]["output"], output) {
		t.Fatalf("structured output changed: %#v", items[0]["output"])
	}
}

func TestResponsesUserMessageItemsPreservesStringFunctionOutput(t *testing.T) {
	items := responsesUserMessageItems(cif.CIFUserMessage{
		Role: "user",
		Content: []cif.CIFContentPart{
			cif.CIFToolResultPart{Type: "tool_result", ToolCallID: "call_1", Content: "ok"},
		},
	}, false)

	if len(items) != 1 || items[0]["output"] != "ok" {
		t.Fatalf("string output changed: %#v", items)
	}
}
