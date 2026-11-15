package antigravity

import (
	"testing"

	"omnillm/internal/cif"
)

func TestToolCallIdentityAndStopSemantics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		finish string
	}{
		{name: "generic stop", finish: "STOP"},
		{name: "native tool stop", finish: "FUNCTION_CALL"},
		{name: "clean EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := map[string]interface{}{
				"content": map[string]interface{}{
					"role": "model",
					"parts": []map[string]interface{}{
						{"functionCall": map[string]interface{}{
							"id": "native-call", "name": "Read", "args": map[string]interface{}{"path": "first"},
						}},
						{"functionCall": map[string]interface{}{
							"name": "Read", "args": map[string]interface{}{"path": "second"},
						}},
					},
				},
			}
			if tc.finish != "" {
				candidate["finishReason"] = tc.finish
			}
			data := buildSSEEvent(map[string]interface{}{
				"response": map[string]interface{}{"candidates": []map[string]interface{}{candidate}},
			})
			events := make(chan cif.CIFStreamEvent, 8)
			ParseAntigravitySSE(t.Context(), newReadCloser(data), events)
			ids := []string{}
			var stop cif.CIFStopReason
			for event := range events {
				switch e := event.(type) {
				case cif.CIFContentDelta:
					if call, ok := e.ContentBlock.(cif.CIFToolCallPart); ok {
						ids = append(ids, call.ToolCallID)
					}
				case cif.CIFStreamEnd:
					stop = e.StopReason
				}
			}
			if len(ids) != 2 || ids[0] != "native-call" || ids[1] == "" || ids[1] == ids[0] {
				t.Fatalf("native/fallback tool IDs = %v", ids)
			}
			if stop != cif.StopReasonToolUse {
				t.Fatalf("tool-call stop reason = %q, want tool_use", stop)
			}
		})
	}
}
