package responsecache

import (
	"testing"

	"omnillm/internal/cif"
)

// feedStream drives an accumulator with a text+toolcall stream and returns the
// assembled response.
func TestStreamAccumulator_TextAndTool(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "id-1", Model: "gpt-x"})
	// Text block at index 0, streamed in two deltas.
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 0, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "Hello "}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 0, Delta: cif.TextDelta{Type: "text_delta", Text: "world"}})
	acc.Observe(cif.CIFContentBlockStop{Type: "content_block_stop", Index: 0})
	// Tool call at index 1, args streamed.
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 1, ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "tc1", ToolName: "search"}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 1, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"q":`}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 1, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `"go"}`}})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse, Usage: &cif.CIFUsage{InputTokens: 5, OutputTokens: 9}})

	resp := acc.Response()
	if resp == nil {
		t.Fatal("expected assembled response, got nil")
	}
	if resp.ID != "id-1" || resp.Model != "gpt-x" || resp.StopReason != cif.StopReasonToolUse {
		t.Fatalf("scalar mismatch: %+v", resp)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(resp.Content))
	}
	txt, ok := resp.Content[0].(cif.CIFTextPart)
	if !ok || txt.Text != "Hello world" {
		t.Fatalf("text accumulation wrong: %#v", resp.Content[0])
	}
	tc, ok := resp.Content[1].(cif.CIFToolCallPart)
	if !ok || tc.ToolName != "search" || tc.ToolArguments["q"] != "go" {
		t.Fatalf("tool accumulation wrong: %#v", resp.Content[1])
	}
}

// TestStreamAccumulator_ContentBlockEveryDelta guards the Copilot-style stream
// where the announcing ContentBlock is attached to EVERY delta, not just the
// first. Re-initializing on each delta would wipe accumulated text (regression).
func TestStreamAccumulator_ContentBlockEveryDelta(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{ID: "id", Model: "m"})
	acc.Observe(cif.CIFContentDelta{Index: 0, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "J"}})
	acc.Observe(cif.CIFContentDelta{Index: 0, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "ACKFRUIT"}})
	acc.Observe(cif.CIFStreamEnd{StopReason: cif.StopReasonEndTurn})
	resp := acc.Response()
	if resp == nil || len(resp.Content) != 1 {
		t.Fatalf("expected 1 content part, got %#v", resp)
	}
	txt := resp.Content[0].(cif.CIFTextPart)
	if txt.Text != "JACKFRUIT" {
		t.Fatalf("ContentBlock-on-every-delta wiped text: got %q, want JACKFRUIT", txt.Text)
	}
}

func TestStreamAccumulator_ErrorAndIncomplete(t *testing.T) {
	// Errored stream ⇒ nil.
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{ID: "x", Model: "m"})
	acc.Observe(cif.CIFContentDelta{Index: 0, Delta: cif.TextDelta{Text: "hi"}})
	acc.Observe(cif.CIFStreamError{Type: "stream_error"})
	if acc.Response() != nil {
		t.Error("errored stream must not be cacheable")
	}

	// Never-ended stream ⇒ nil.
	acc2 := NewStreamAccumulator()
	acc2.Observe(cif.CIFStreamStart{ID: "x", Model: "m"})
	acc2.Observe(cif.CIFContentDelta{Index: 0, Delta: cif.TextDelta{Text: "hi"}})
	if acc2.Response() != nil {
		t.Error("stream without end must not be cacheable")
	}
}

// TestStreamRoundTrip verifies accumulate → synthesize → re-accumulate is stable.
func TestStreamRoundTrip(t *testing.T) {
	orig := &cif.CanonicalResponse{
		ID:         "r1",
		Model:      "gpt-x",
		StopReason: cif.StopReasonEndTurn,
		Usage:      &cif.CIFUsage{InputTokens: 3, OutputTokens: 7},
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "the answer is 42"},
		},
	}
	events := SynthesizeStream(orig)

	acc := NewStreamAccumulator()
	for _, ev := range events {
		acc.Observe(ev)
	}
	got := acc.Response()
	if got == nil {
		t.Fatal("round-trip produced nil")
	}
	if got.ID != orig.ID || got.Model != orig.Model || got.StopReason != orig.StopReason {
		t.Fatalf("scalar mismatch after round-trip: %+v", got)
	}
	if len(got.Content) != 1 {
		t.Fatalf("expected 1 part, got %d", len(got.Content))
	}
	txt, ok := got.Content[0].(cif.CIFTextPart)
	if !ok || txt.Text != "the answer is 42" {
		t.Fatalf("text lost in round-trip: %#v", got.Content[0])
	}
	// Synthesized events must start with stream_start and end with stream_end.
	if events[0].GetEventType() != "stream_start" {
		t.Errorf("first event should be stream_start, got %s", events[0].GetEventType())
	}
	if events[len(events)-1].GetEventType() != "stream_end" {
		t.Errorf("last event should be stream_end, got %s", events[len(events)-1].GetEventType())
	}
}
