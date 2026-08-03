package routes

import (
	"testing"

	"omnillm/internal/cif"
)

func TestNormalizeCachedToolArgumentsRepairsLegacyEntry(t *testing.T) {
	legacy := &cif.CanonicalResponse{Content: []cif.CIFContentPart{
		cif.CIFToolCallPart{
			Type: "tool_call", ToolCallID: "call", ToolName: "search",
			ToolArguments: map[string]interface{}{"query": "term", "channel": ""},
		},
	}}
	request := &cif.CanonicalRequest{Tools: []cif.CIFTool{{
		Name: "search",
		ParametersSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string"}, "channel": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"query"},
		},
	}}}

	normalized := normalizeCachedToolArguments(legacy, request)
	arguments := normalized.Content[0].(cif.CIFToolCallPart).ToolArguments
	if arguments["query"] != "term" {
		t.Fatalf("required query lost: %#v", arguments)
	}
	if _, exists := arguments["channel"]; exists {
		t.Fatalf("legacy optional empty argument survived: %#v", arguments)
	}
	if legacy.Content[0].(cif.CIFToolCallPart).ToolArguments["channel"] != "" {
		t.Fatal("legacy cache object was mutated")
	}
}
