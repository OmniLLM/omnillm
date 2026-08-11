package shared

import (
	"encoding/json"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/testcompat"
)

func TestSynthesizedCompatibilityStreamIsSingleAndCoherent(t *testing.T) {
	scenario := testcompat.AgenticScenario()
	response := testcompat.Response(scenario)
	counts := map[string]int{}
	arguments := map[string]string{}
	terminal := 0

	for event := range StreamResponse(response) {
		switch typed := event.(type) {
		case cif.CIFStreamStart:
			counts["start"]++
		case cif.CIFContentDelta:
			if call, ok := typed.ContentBlock.(cif.CIFToolCallPart); ok {
				counts[call.ToolCallID]++
				if delta, ok := typed.Delta.(cif.ToolArgumentsDelta); ok {
					arguments[call.ToolCallID] += delta.PartialJSON
				}
			}
		case cif.CIFStreamEnd:
			terminal++
		}
	}

	if counts["start"] != 1 || terminal != 1 {
		t.Fatalf("stream boundaries: starts=%d terminals=%d", counts["start"], terminal)
	}
	for _, exchange := range scenario.Exchanges {
		if counts[exchange.ID] != 1 {
			t.Errorf("tool call %s emitted %d times", exchange.ID, counts[exchange.ID])
		}
		var got map[string]interface{}
		if err := json.Unmarshal([]byte(arguments[exchange.ID]), &got); err != nil {
			t.Errorf("tool call %s arguments invalid: %v", exchange.ID, err)
		}
		if got["file_path"] != exchange.Arguments["file_path"] {
			t.Errorf("tool call %s arguments = %#v, want %#v", exchange.ID, got, exchange.Arguments)
		}
	}
}
