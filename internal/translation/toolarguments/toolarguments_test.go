package toolarguments

import (
	"context"
	"reflect"
	"testing"
	"time"

	"omnillm/internal/cif"
)

func objectSchema(properties map[string]interface{}, required ...interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

func TestNormalizeMap(t *testing.T) {
	nested := map[string]interface{}{"child": ""}
	tests := []struct {
		name   string
		args   map[string]interface{}
		schema map[string]interface{}
		want   map[string]interface{}
	}{
		{"optional empty omitted", map[string]interface{}{"optional": "", "required": ""}, objectSchema(map[string]interface{}{"optional": map[string]interface{}{"type": "string"}, "required": map[string]interface{}{"type": "string"}}, "required"), map[string]interface{}{"required": ""}},
		{"nonempty preserved", map[string]interface{}{"optional": "value"}, objectSchema(map[string]interface{}{"optional": map[string]interface{}{"type": "string"}}), map[string]interface{}{"optional": "value"}},
		{"non-string values preserved", map[string]interface{}{"zero": 0.0, "false": false, "null": nil, "array": []interface{}{}, "object": map[string]interface{}{}}, objectSchema(map[string]interface{}{"zero": map[string]interface{}{}, "false": map[string]interface{}{}, "null": map[string]interface{}{}, "array": map[string]interface{}{}, "object": map[string]interface{}{}}), map[string]interface{}{"zero": 0.0, "false": false, "null": nil, "array": []interface{}{}, "object": map[string]interface{}{}}},
		{"nested empty preserved", map[string]interface{}{"nested": nested}, objectSchema(map[string]interface{}{"nested": map[string]interface{}{"type": "object"}}), map[string]interface{}{"nested": nested}},
		{"undeclared empty preserved", map[string]interface{}{"unknown": ""}, objectSchema(map[string]interface{}{}), map[string]interface{}{"unknown": ""}},
		{"missing schema preserved", map[string]interface{}{"optional": ""}, nil, map[string]interface{}{"optional": ""}},
		{"malformed required preserved", map[string]interface{}{"optional": ""}, map[string]interface{}{"type": "object", "properties": map[string]interface{}{"optional": map[string]interface{}{}}, "required": "optional"}, map[string]interface{}{"optional": ""}},
		{"non-object schema preserved", map[string]interface{}{"optional": ""}, map[string]interface{}{"type": "string", "properties": map[string]interface{}{"optional": map[string]interface{}{}}}, map[string]interface{}{"optional": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeMap(tt.args, tt.schema)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NormalizeMap() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeJSONPreservesUncertainBytes(t *testing.T) {
	schema := objectSchema(map[string]interface{}{"optional": map[string]interface{}{"type": "string"}})
	tests := []string{`{"optional":`, `[]`, `null`, ` { "other" : "value" } `}
	for _, input := range tests {
		if got := NormalizeJSON(input, schema); got != input {
			t.Errorf("NormalizeJSON(%q) = %q, want byte-for-byte preservation", input, got)
		}
	}
	if got := NormalizeJSON(` { "optional" : "", "keep" : "x" } `, schema); got != `{"keep":"x"}` {
		t.Fatalf("normalized JSON = %q", got)
	}
}

func TestNormalizeResponseCopyOnWrite(t *testing.T) {
	arguments := map[string]interface{}{"optional": "", "keep": "x"}
	response := &cif.CanonicalResponse{Content: []cif.CIFContentPart{
		cif.CIFTextPart{Type: "text", Text: "before"},
		cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call", ToolName: "tool", ToolArguments: arguments},
	}}
	tools := []cif.CIFTool{{Name: "tool", ParametersSchema: objectSchema(map[string]interface{}{"optional": map[string]interface{}{}, "keep": map[string]interface{}{}})}}

	got := NormalizeResponse(response, tools)
	if got == response {
		t.Fatal("expected changed response copy")
	}
	tool := got.Content[1].(cif.CIFToolCallPart)
	if _, exists := tool.ToolArguments["optional"]; exists || tool.ToolArguments["keep"] != "x" {
		t.Fatalf("normalized arguments = %#v", tool.ToolArguments)
	}
	if arguments["optional"] != "" {
		t.Fatalf("original arguments mutated: %#v", arguments)
	}

	unchanged := &cif.CanonicalResponse{Content: []cif.CIFContentPart{cif.CIFToolCallPart{ToolName: "unknown", ToolArguments: arguments}}}
	if NormalizeResponse(unchanged, tools) != unchanged {
		t.Fatal("unchanged response should retain identity")
	}
}

func TestNormalizeJSONPreservesLargeInteger(t *testing.T) {
	schema := objectSchema(map[string]interface{}{"optional": map[string]interface{}{}, "integer": map[string]interface{}{}})
	got := NormalizeJSON(`{"integer":9007199254740993,"optional":""}`, schema)
	if got != `{"integer":9007199254740993}` {
		t.Fatalf("large integer changed: %s", got)
	}
}

func TestNormalizeStreamUnknownToolPassesChunksThrough(t *testing.T) {
	input := make(chan cif.CIFStreamEvent, 8)
	out := NormalizeStream(context.Background(), input, []cif.CIFTool{{Name: "known", ParametersSchema: nil}})
	original := []cif.CIFStreamEvent{
		toolAnnouncement(3, "call", "unknown"),
		argumentDelta(3, `{"a":`),
		cif.CIFContentDelta{Type: "content_delta", Index: 1, Delta: cif.TextDelta{Type: "text_delta", Text: "between"}},
		argumentDelta(3, `1}`),
		cif.CIFContentBlockStop{Type: "content_block_stop", Index: 3},
	}
	for _, event := range original {
		input <- event
	}
	close(input)
	var got []cif.CIFStreamEvent
	for event := range out {
		got = append(got, event)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("pass-through events changed:\n got %#v\nwant %#v", got, original)
	}
}

func TestNormalizeStreamInterleavedAndRepeatedAnnouncements(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	input := make(chan cif.CIFStreamEvent, 16)
	tools := []cif.CIFTool{
		{Name: "first", ParametersSchema: objectSchema(map[string]interface{}{"optional": map[string]interface{}{}, "required": map[string]interface{}{}}, "required")},
		{Name: "second", ParametersSchema: objectSchema(map[string]interface{}{"optional": map[string]interface{}{}, "value": map[string]interface{}{}})},
	}
	out := NormalizeStream(ctx, input, tools)
	input <- cif.CIFStreamStart{Type: "stream_start"}
	input <- toolAnnouncement(0, "a", "first")
	input <- toolAnnouncement(1, "b", "second")
	input <- argumentDelta(0, `{"optional":"",`)
	input <- toolAnnouncement(0, "a", "first")
	input <- argumentDelta(1, `{"optional":"",`)
	input <- argumentDelta(0, `"required":""}`)
	input <- argumentDelta(1, `"value":"ok"}`)
	input <- cif.CIFContentBlockStop{Type: "content_block_stop", Index: 1}
	input <- cif.CIFContentBlockStop{Type: "content_block_stop", Index: 0}
	input <- cif.CIFStreamEnd{Type: "stream_end"}
	close(input)

	var got []cif.CIFStreamEvent
	for event := range out {
		got = append(got, event)
	}
	var deltas []cif.CIFContentDelta
	for _, event := range got {
		if delta, ok := event.(cif.CIFContentDelta); ok {
			if _, ok := delta.Delta.(cif.ToolArgumentsDelta); ok {
				deltas = append(deltas, delta)
			}
		}
	}
	if len(deltas) != 2 {
		t.Fatalf("got %d argument deltas: %#v", len(deltas), got)
	}
	if deltas[0].Index != 1 || deltas[0].Delta.(cif.ToolArgumentsDelta).PartialJSON != `{"value":"ok"}` {
		t.Errorf("first flushed delta = %#v", deltas[0])
	}
	if deltas[1].Index != 0 || deltas[1].Delta.(cif.ToolArgumentsDelta).PartialJSON != `{"required":""}` {
		t.Errorf("second flushed delta = %#v", deltas[1])
	}
}

func TestNormalizeStreamFlushesBeforeIndexReuseAndClose(t *testing.T) {
	input := make(chan cif.CIFStreamEvent, 8)
	tools := []cif.CIFTool{{Name: "tool", ParametersSchema: objectSchema(map[string]interface{}{"optional": map[string]interface{}{}})}}
	out := NormalizeStream(context.Background(), input, tools)
	input <- toolAnnouncement(0, "a", "tool")
	input <- argumentDelta(0, `{"optional":""}`)
	input <- toolAnnouncement(0, "b", "tool")
	input <- argumentDelta(0, `{"keep":"x"}`)
	close(input)

	var args []string
	for event := range out {
		if delta, ok := event.(cif.CIFContentDelta); ok {
			if value, ok := delta.Delta.(cif.ToolArgumentsDelta); ok {
				args = append(args, value.PartialJSON)
			}
		}
	}
	if !reflect.DeepEqual(args, []string{"{}", `{"keep":"x"}`}) {
		t.Fatalf("flushed arguments = %#v", args)
	}
}

func TestNormalizeStreamFlushesBeforeContentKindReuse(t *testing.T) {
	input := make(chan cif.CIFStreamEvent, 4)
	out := NormalizeStream(context.Background(), input, []cif.CIFTool{{
		Name: "tool", ParametersSchema: objectSchema(map[string]interface{}{"optional": map[string]interface{}{}}),
	}})
	input <- toolAnnouncement(2, "call", "tool")
	input <- argumentDelta(2, `{"value":"x"}`)
	input <- cif.CIFContentDelta{Type: "content_delta", Index: 2, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "done"}}
	close(input)

	var got []cif.CIFStreamEvent
	for event := range out {
		got = append(got, event)
	}
	if len(got) != 4 {
		t.Fatalf("events = %#v", got)
	}
	flushed, ok := got[2].(cif.CIFContentDelta)
	if !ok || flushed.Delta.(cif.ToolArgumentsDelta).PartialJSON != `{"value":"x"}` {
		t.Fatalf("tool arguments were not flushed before content reuse: %#v", got)
	}
	if text, ok := got[3].(cif.CIFContentDelta); !ok || text.Delta.(cif.TextDelta).Text != "done" {
		t.Fatalf("text announcement did not follow flush: %#v", got)
	}
}

func TestNormalizeStreamRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input := make(chan cif.CIFStreamEvent)
	out := NormalizeStream(ctx, input, nil)
	cancel()
	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("normalizer did not stop on cancellation")
	}
}

func toolAnnouncement(index int, id, name string) cif.CIFContentDelta {
	return cif.CIFContentDelta{Type: "content_delta", Index: index, ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: id, ToolName: name}, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta"}}
}

func argumentDelta(index int, partial string) cif.CIFContentDelta {
	return cif.CIFContentDelta{Type: "content_delta", Index: index, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: partial}}
}
