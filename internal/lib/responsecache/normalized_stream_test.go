package responsecache

import (
	"context"
	"encoding/json"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/translation/toolarguments"
)

func TestStreamAccumulatorSplitsStoppedAndIdentityReusedBlocks(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r", Model: "m"})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 1, ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "a", ToolName: "first"}, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"a":1}`}})
	acc.Observe(cif.CIFContentBlockStop{Type: "content_block_stop", Index: 1})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 1, ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "b", ToolName: "second"}, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"b":2}`}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 1, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "done"}})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn})

	response := acc.Response()
	if response == nil || len(response.Content) != 3 {
		t.Fatalf("accumulated response = %#v", response)
	}
	first := response.Content[0].(cif.CIFToolCallPart)
	second := response.Content[1].(cif.CIFToolCallPart)
	text := response.Content[2].(cif.CIFTextPart)
	if first.ToolCallID != "a" || first.ToolArguments["a"] != json.Number("1") || second.ToolCallID != "b" || second.ToolArguments["b"] != json.Number("2") || text.Text != "done" {
		t.Fatalf("split blocks = %#v", response.Content)
	}
	if replayed := SynthesizeStream(response); len(replayed) != 8 {
		t.Fatalf("replayed event count = %d, want 8", len(replayed))
	}
}

func TestNormalizedStreamPreservesLargeIntegerOnCacheReplay(t *testing.T) {
	input := make(chan cif.CIFStreamEvent, 5)
	tools := []cif.CIFTool{{
		Name: "lookup",
		ParametersSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":       map[string]interface{}{"type": "integer"},
				"optional": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"id"},
		},
	}}
	normalized := toolarguments.NormalizeStream(context.Background(), input, tools)
	input <- cif.CIFStreamStart{Type: "stream_start", ID: "r1", Model: "m"}
	input <- cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        0,
		ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call", ToolName: "lookup"},
		Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"id":9007199254740993,"optional":""}`},
	}
	input <- cif.CIFContentBlockStop{Type: "content_block_stop", Index: 0}
	input <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse}
	close(input)

	acc := NewStreamAccumulator()
	var live string
	for event := range normalized {
		acc.Observe(event)
		if delta, ok := event.(cif.CIFContentDelta); ok {
			if args, ok := delta.Delta.(cif.ToolArgumentsDelta); ok {
				live += args.PartialJSON
			}
		}
	}
	if live != `{"id":9007199254740993}` {
		t.Fatalf("live arguments = %q", live)
	}

	response := acc.Response()
	if response == nil {
		t.Fatal("expected cached response")
	}
	var replayed string
	for _, event := range SynthesizeStream(response) {
		if delta, ok := event.(cif.CIFContentDelta); ok {
			if args, ok := delta.Delta.(cif.ToolArgumentsDelta); ok {
				replayed += args.PartialJSON
			}
		}
	}
	if replayed != live {
		t.Fatalf("replayed arguments = %q, live = %q", replayed, live)
	}
}

func TestNormalizedStreamToolArgumentsSurviveCacheReplay(t *testing.T) {
	input := make(chan cif.CIFStreamEvent, 8)
	tools := []cif.CIFTool{{
		Name: "search",
		ParametersSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":   map[string]interface{}{"type": "string"},
				"channel": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"query"},
		},
	}}
	normalized := toolarguments.NormalizeStream(context.Background(), input, tools)
	input <- cif.CIFStreamStart{Type: "stream_start", ID: "r1", Model: "m"}
	input <- cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        4,
		ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call", ToolName: "search"},
		Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"query":"term",`},
	}
	input <- cif.CIFContentDelta{Type: "content_delta", Index: 4, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `"channel":""}`}}
	input <- cif.CIFContentBlockStop{Type: "content_block_stop", Index: 4}
	input <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse}
	close(input)

	acc := NewStreamAccumulator()
	var live string
	for event := range normalized {
		acc.Observe(event)
		if delta, ok := event.(cif.CIFContentDelta); ok {
			if args, ok := delta.Delta.(cif.ToolArgumentsDelta); ok {
				live += args.PartialJSON
			}
		}
	}
	if live != `{"query":"term"}` {
		t.Fatalf("live arguments = %q", live)
	}

	response := acc.Response()
	if response == nil {
		t.Fatal("expected cached response")
	}
	var replayed string
	for _, event := range SynthesizeStream(response) {
		if delta, ok := event.(cif.CIFContentDelta); ok {
			if args, ok := delta.Delta.(cif.ToolArgumentsDelta); ok {
				replayed += args.PartialJSON
			}
		}
	}
	if replayed != live {
		t.Fatalf("replayed arguments = %q, live = %q", replayed, live)
	}
}
