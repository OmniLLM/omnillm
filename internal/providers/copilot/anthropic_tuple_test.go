package copilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
)

func TestAnthropicLegacyTupleCopilotContinuation(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests++
		tools := payload["tools"].([]interface{})
		fn := tools[0].(map[string]interface{})["function"].(map[string]interface{})
		schema := fn["parameters"].(map[string]interface{})
		rgb := schema["properties"].(map[string]interface{})["baseColor"].(map[string]interface{})
		prefix, ok := rgb["prefixItems"].([]interface{})
		if !ok || len(prefix) != 3 || rgb["items"] != false {
			t.Errorf("upstream tuple = %#v", rgb)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, position := range prefix {
			if !reflect.DeepEqual(position, map[string]interface{}{"type": "integer", "minimum": float64(0), "maximum": float64(255)}) {
				t.Errorf("RGB constraint changed: %#v", position)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = w.Write([]byte(`{"id":"turn1","choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"rgb1","type":"function","function":{"name":"color_harmony","arguments":"{\"baseColor\":[1,2,3]}"}}]},"finish_reason":"tool_calls"}]}`))
			return
		}
		messages := payload["messages"].([]interface{})
		last := messages[len(messages)-1].(map[string]interface{})
		if last["role"] != "tool" || last["tool_call_id"] != "rgb1" || last["content"] != "observed" {
			t.Errorf("tool result relationship changed: %#v", last)
		}
		_, _ = w.Write([]byte(`{"id":"turn2","choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()
	adapter := newTestAdapter(t, upstream.URL)
	tool := `{"name":"color_harmony","input_schema":{"type":"object","properties":{"baseColor":{"type":"array","items":[{"type":"integer","minimum":0,"maximum":255},{"type":"integer","minimum":0,"maximum":255},{"type":"integer","minimum":0,"maximum":255}],"additionalItems":false}},"required":["baseColor"]}}`
	history := `[{"role":"user","content":"Use RGB"}]`
	for turn := range 2 {
		req, err := ingestion.ParseAnthropicMessages(json.RawMessage(`{"model":"claude-sonnet-5.5","max_tokens":32,"messages":` + history + `,"tools":[` + tool + `]}`))
		if err != nil {
			t.Fatal(err)
		}
		response, err := adapter.Execute(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if turn == 0 {
			call, ok := response.Content[0].(cif.CIFToolCallPart)
			if !ok || call.ToolCallID != "rgb1" || call.ToolName != "color_harmony" || response.StopReason != cif.StopReasonToolUse {
				t.Fatalf("tool call changed: %#v", response)
			}
			history = `[{"role":"user","content":"Use RGB"},{"role":"assistant","content":[{"type":"tool_use","id":"rgb1","name":"color_harmony","input":{"baseColor":[1,2,3]}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"rgb1","content":"observed"}]}]`
		} else if response.StopReason != cif.StopReasonEndTurn || response.Content[0].(cif.CIFTextPart).Text != "done" {
			t.Fatalf("continuation did not terminate: %#v", response)
		}
	}
	if requests != 2 {
		t.Fatalf("upstream requests = %d, want 2", requests)
	}
}
