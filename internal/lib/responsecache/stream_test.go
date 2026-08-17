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

func TestStreamAccumulator_CustomToolInput(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r-custom", Model: "m"})
	acc.Observe(cif.CIFContentDelta{
		Type:  "content_delta",
		Index: 3,
		ContentBlock: cif.CIFToolCallPart{
			Type:       "tool_call",
			ToolCallID: "custom-1",
			ToolName:   "shell",
			ToolKind:   cif.CIFToolKindCustom,
			Namespace:  "computer",
		},
		Delta: cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: "line one\n"},
	})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 3, Delta: cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: "line two"}})
	acc.Observe(cif.CIFContentBlockStop{Type: "content_block_stop", Index: 3})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	resp := acc.Response()
	if resp == nil || len(resp.Content) != 1 {
		t.Fatalf("custom response = %#v", resp)
	}
	call := resp.Content[0].(cif.CIFToolCallPart)
	want := "line one\nline two"
	if call.ToolKind != cif.CIFToolKindCustom || call.ToolCallID != "custom-1" || call.ToolName != "shell" || call.Namespace != "computer" || call.RawInput == nil || *call.RawInput != want || call.ToolArguments["input"] != want {
		t.Fatalf("custom call = %#v, want raw input %q", call, want)
	}
}

func TestStreamAccumulator_CustomAnnouncementInputWithoutDeltas(t *testing.T) {
	input := "complete announced input"
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r", Model: "m"})
	acc.Observe(cif.CIFContentDelta{
		Type: "content_delta", Index: 0,
		ContentBlock: cif.CIFToolCallPart{
			Type: "tool_call", ToolCallID: "custom", ToolName: "shell",
			ToolKind: cif.CIFToolKindCustom, RawInput: &input,
			ToolArguments: map[string]interface{}{"input": input},
		},
		Delta: cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: ""},
	})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	call := acc.Response().Content[0].(cif.CIFToolCallPart)
	if call.RawInput == nil || *call.RawInput != input || call.ToolArguments["input"] != input {
		t.Fatalf("announced input lost: %#v", call)
	}
}

func TestStreamAccumulator_ExplicitEmptyCustomInput(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r", Model: "m"})
	acc.Observe(cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        0,
		ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "custom", ToolName: "shell", ToolKind: cif.CIFToolKindCustom},
		Delta:        cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: ""},
	})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	call := acc.Response().Content[0].(cif.CIFToolCallPart)
	if call.RawInput == nil || *call.RawInput != "" {
		t.Fatalf("RawInput = %#v; want present empty input", call.RawInput)
	}
	if input, ok := call.ToolArguments["input"]; !ok || input != "" {
		t.Fatalf("fallback arguments = %#v; want present empty input", call.ToolArguments)
	}
}

func TestStreamAccumulator_InterleavedFunctionAndCustomCalls(t *testing.T) {
	acc := NewStreamAccumulator()
	acc.Observe(cif.CIFStreamStart{Type: "stream_start", ID: "r", Model: "m"})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 7, ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "function", ToolName: "lookup"}, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"q":`}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 2, ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "custom", ToolName: "shell", ToolKind: cif.CIFToolKindCustom, Namespace: "computer"}, Delta: cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: "ec"}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 7, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `"go"}`}})
	acc.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 2, Delta: cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: "ho hi"}})
	acc.Observe(cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse})

	resp := acc.Response()
	if resp == nil || len(resp.Content) != 2 {
		t.Fatalf("response = %#v", resp)
	}
	function := resp.Content[0].(cif.CIFToolCallPart)
	custom := resp.Content[1].(cif.CIFToolCallPart)
	if function.ToolCallID != "function" || function.ToolArguments["q"] != "go" || function.RawInput != nil {
		t.Fatalf("function call mixed vocabularies: %#v", function)
	}
	if custom.ToolCallID != "custom" || custom.RawInput == nil || *custom.RawInput != "echo hi" || len(custom.ToolArguments) != 1 {
		t.Fatalf("custom call mixed vocabularies: %#v", custom)
	}
}

func TestSynthesizeStream_CustomToolVocabularyAndTerminal(t *testing.T) {
	empty := ""
	input := "line one\nline two"
	resp := &cif.CanonicalResponse{
		ID: "r", Model: "m", StopReason: cif.StopReasonToolUse,
		Content: []cif.CIFContentPart{
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "function", ToolName: "lookup", ToolArguments: map[string]interface{}{"q": "go"}},
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "custom", ToolName: "shell", ToolKind: cif.CIFToolKindCustom, RawInput: &input, Namespace: "computer", ToolArguments: map[string]interface{}{"input": input}},
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "empty", ToolName: "shell", ToolKind: cif.CIFToolKindCustom, RawInput: &empty},
		},
	}

	events := SynthesizeStream(resp)
	var functionDeltas, customDeltas, stops, terminals int
	for _, event := range events {
		switch typed := event.(type) {
		case cif.CIFContentDelta:
			announcement, ok := typed.ContentBlock.(cif.CIFToolCallPart)
			if !ok {
				t.Fatalf("tool delta lacks announcement: %#v", typed)
			}
			switch delta := typed.Delta.(type) {
			case cif.ToolArgumentsDelta:
				functionDeltas++
				if announcement.ToolKind != "" || delta.PartialJSON != `{"q":"go"}` {
					t.Fatalf("function replay = %#v", typed)
				}
			case cif.CustomToolInputDelta:
				customDeltas++
				if announcement.ToolKind != cif.CIFToolKindCustom || announcement.RawInput == nil || *announcement.RawInput != "" {
					t.Fatalf("custom announcement = %#v", announcement)
				}
				if announcement.ToolCallID == "custom" && delta.Delta != input {
					t.Fatalf("custom input delta = %q, want %q", delta.Delta, input)
				}
				if announcement.ToolCallID == "empty" && delta.Delta != "" {
					t.Fatalf("empty custom delta = %q", delta.Delta)
				}
			}
		case cif.CIFContentBlockStop:
			stops++
		case cif.CIFStreamEnd:
			terminals++
		}
	}
	if functionDeltas != 1 || customDeltas != 2 || stops != 3 || terminals != 1 {
		t.Fatalf("event counts function=%d custom=%d stops=%d terminals=%d", functionDeltas, customDeltas, stops, terminals)
	}

	replayed := NewStreamAccumulator()
	for _, event := range events {
		replayed.Observe(event)
	}
	got := replayed.Response()
	if got == nil || len(got.Content) != 3 {
		t.Fatalf("replayed response = %#v", got)
	}
	for i, want := range resp.Content {
		gotCall := got.Content[i].(cif.CIFToolCallPart)
		wantCall := want.(cif.CIFToolCallPart)
		if gotCall.ToolCallID != wantCall.ToolCallID || gotCall.ToolKind != wantCall.ToolKind || gotCall.Namespace != wantCall.Namespace {
			t.Fatalf("call %d metadata = %#v, want %#v", i, gotCall, wantCall)
		}
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
