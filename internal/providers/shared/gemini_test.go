package shared

import (
	"testing"

	"omnillm/internal/cif"
)

func TestCIFMessagesToGeminiUsesStructuredOutputFallbackText(t *testing.T) {
	const fallback = `[{"type":"input_text","text":"Structured"}]`
	contents := CIFMessagesToGemini([]cif.CIFMessage{
		cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
			cif.CIFToolResultPart{
				Type:       "tool_result",
				ToolCallID: "call_structured",
				ToolName:   "Read",
				Content:    fallback,
				RawOutput:  []interface{}{map[string]interface{}{"type": "input_text", "text": "Structured"}},
			},
		}},
	})
	if len(contents) != 1 {
		t.Fatalf("contents = %#v", contents)
	}
	parts := contents[0]["parts"].([]map[string]interface{})
	response := parts[0]["functionResponse"].(map[string]interface{})["response"].(map[string]interface{})
	if response["output"] != fallback {
		t.Fatalf("Gemini output = %#v", response["output"])
	}
}
