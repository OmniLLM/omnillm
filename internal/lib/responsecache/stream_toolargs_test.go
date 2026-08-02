package responsecache

import (
	"testing"

	"omnillm/internal/cif"
)

// Providers announce a tool call with an empty-but-non-nil ToolArguments map
// and stream the real arguments as deltas. Seeding the accumulator with that
// empty map made it win over the accumulated rawArgs in Response(), so every
// cached streaming tool call replayed with no arguments at all.
func TestStreamAccumulator_ToolArgsFromDeltasNotEmptyAnnouncement(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r1", Model: "m"})
	acc.Observe(cif.CIFContentDelta{
		Type: "content_delta", Index: 0,
		ContentBlock: cif.CIFToolCallPart{
			Type: "tool_call", ToolCallID: "c1", ToolName: "Bash",
			ToolArguments: map[string]interface{}{},
		},
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""},
	})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 0,
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"command":`}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 0,
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `"ls -la"}`}})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	resp := acc.Response()
	if resp == nil {
		t.Fatal("expected a response")
	}
	tc, ok := resp.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected a tool call part, got %T", resp.Content[0])
	}
	if got := tc.ToolArguments["command"]; got != "ls -la" {
		t.Fatalf("streamed arguments were discarded: ToolArguments = %#v, want command=%q", tc.ToolArguments, "ls -la")
	}
	if tc.ToolName != "Bash" || tc.ToolCallID != "c1" {
		t.Errorf("tool identity lost: name=%q id=%q", tc.ToolName, tc.ToolCallID)
	}
}

// A non-streaming-style announcement that carries complete arguments and emits
// no argument deltas must still be preserved.
func TestStreamAccumulator_ToolArgsFromAnnouncementWhenNoDeltas(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r1", Model: "m"})
	acc.Observe(cif.CIFContentDelta{
		Type: "content_delta", Index: 0,
		ContentBlock: cif.CIFToolCallPart{
			Type: "tool_call", ToolCallID: "c1", ToolName: "Read",
			ToolArguments: map[string]interface{}{"path": "/tmp/a"},
		},
	})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	resp := acc.Response()
	if resp == nil {
		t.Fatal("expected a response")
	}
	tc := resp.Content[0].(cif.CIFToolCallPart)
	if tc.ToolArguments["path"] != "/tmp/a" {
		t.Fatalf("announced arguments lost: %#v", tc.ToolArguments)
	}
}

// Two concurrent tool calls streaming interleaved deltas must accumulate into
// separate blocks with their own arguments.
func TestStreamAccumulator_InterleavedToolCalls(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r1", Model: "m"})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 0,
		ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "a", ToolName: "Read", ToolArguments: map[string]interface{}{}}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 1,
		ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "b", ToolName: "Bash", ToolArguments: map[string]interface{}{}}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 0,
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"path":"/a"}`}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 1,
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"command":"ls"}`}})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	resp := acc.Response()
	if resp == nil || len(resp.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %+v", resp)
	}
	byName := map[string]map[string]interface{}{}
	for _, p := range resp.Content {
		tc := p.(cif.CIFToolCallPart)
		byName[tc.ToolName] = tc.ToolArguments
	}
	if byName["Read"]["path"] != "/a" {
		t.Errorf("Read arguments = %#v, want path=/a", byName["Read"])
	}
	if byName["Bash"]["command"] != "ls" {
		t.Errorf("Bash arguments = %#v, want command=ls", byName["Bash"])
	}
}

func TestStreamAccumulator_DeltasOverrideAnnouncedArgs(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r1", Model: "m"})
	acc.Observe(cif.CIFContentDelta{
		Type: "content_delta", Index: 0,
		ContentBlock: cif.CIFToolCallPart{
			Type: "tool_call", ToolCallID: "c1", ToolName: "Bash",
			ToolArguments: map[string]interface{}{"command": "stale"},
		},
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"command":"real"}`},
	})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	resp := acc.Response()
	if resp == nil {
		t.Fatal("expected a response")
	}
	tc := resp.Content[0].(cif.CIFToolCallPart)
	if got := tc.ToolArguments["command"]; got != "real" {
		t.Fatalf("delta arguments did not override announced arguments: got %q", got)
	}
}

func TestStreamAccumulator_ReusedIndexKeepsBothBlockTypes(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r1", Model: "m"})
	acc.Observe(cif.CIFContentDelta{
		Type: "content_delta", Index: 1,
		ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "c1", ToolName: "Bash"},
		Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"command":"ls"}`},
	})
	acc.Observe(cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        1,
		ContentBlock: cif.CIFTextPart{Type: "text"},
		Delta:        cif.TextDelta{Type: "text_delta", Text: "done"},
	})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn})

	resp := acc.Response()
	if resp == nil || len(resp.Content) != 2 {
		t.Fatalf("expected tool and text blocks, got %+v", resp)
	}
	tool, ok := resp.Content[0].(cif.CIFToolCallPart)
	if !ok || tool.ToolArguments["command"] != "ls" {
		t.Fatalf("first block = %#v", resp.Content[0])
	}
	text, ok := resp.Content[1].(cif.CIFTextPart)
	if !ok || text.Text != "done" {
		t.Fatalf("second block = %#v", resp.Content[1])
	}
}

func TestStreamAccumulator_ToolArgsSurviveCacheReplay(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r1", Model: "m"})
	acc.Observe(cif.CIFContentDelta{
		Type: "content_delta", Index: 0,
		ContentBlock: cif.CIFToolCallPart{
			Type: "tool_call", ToolCallID: "slack-1", ToolName: "mcp__slackBlizzard__slack_search_public_and_private",
			ToolArguments: map[string]interface{}{},
		},
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"query":"cn_it_`},
	})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 0,
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `production","limit":20}`}})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	resp := acc.Response()
	if resp == nil {
		t.Fatal("expected accumulated response")
	}
	replayed := SynthesizeStream(resp)

	var got string
	for _, event := range replayed {
		delta, ok := event.(cif.CIFContentDelta)
		if !ok {
			continue
		}
		args, ok := delta.Delta.(cif.ToolArgumentsDelta)
		if ok {
			got += args.PartialJSON
		}
	}
	want := `{"limit":20,"query":"cn_it_production"}`
	if got != want {
		t.Fatalf("replayed tool arguments = %q, want %q", got, want)
	}
}
