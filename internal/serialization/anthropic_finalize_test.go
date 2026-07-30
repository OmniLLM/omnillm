package serialization

import (
	"strings"
	"testing"

	"omnillm/internal/cif"
)

// TestFinalizeAnthropicStreamClosesOpenBlock verifies that a stream truncated
// mid content block is closed with content_block_stop + message_delta +
// message_stop, which is what prevents clients from reporting
// "Connection closed mid-response".
func TestFinalizeAnthropicStreamClosesOpenBlock(t *testing.T) {
	state := CreateAnthropicStreamState()

	if _, err := ConvertCIFEventToAnthropicSSE(cif.CIFStreamStart{
		Type: "stream_start", ID: "msg_1", Model: "m",
	}, state); err != nil {
		t.Fatalf("stream start: %v", err)
	}
	if _, err := ConvertCIFEventToAnthropicSSE(cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        0,
		ContentBlock: cif.CIFTextPart{Type: "text"},
		Delta:        cif.TextDelta{Type: "text_delta", Text: "partial"},
	}, state); err != nil {
		t.Fatalf("content delta: %v", err)
	}

	if !state.ContentBlockOpen {
		t.Fatal("expected an open content block before finalize")
	}

	events := FinalizeAnthropicStream(state, cif.StopReasonEndTurn)
	if len(events) != 3 {
		t.Fatalf("expected 3 terminal events, got %d: %v", len(events), events)
	}

	var types []string
	for _, e := range events {
		et, _ := e["type"].(string)
		types = append(types, et)
	}
	want := "content_block_stop,message_delta,message_stop"
	if got := strings.Join(types, ","); got != want {
		t.Errorf("terminal event sequence = %q, want %q", got, want)
	}

	if state.ContentBlockOpen {
		t.Error("content block should be closed after finalize")
	}

	// Finalizing twice must not double-emit terminal events.
	if again := FinalizeAnthropicStream(state, cif.StopReasonEndTurn); len(again) != 0 {
		t.Errorf("second finalize emitted %d events, want 0", len(again))
	}
}

// TestFinalizeAnthropicStreamNoMessageStart verifies that nothing is emitted if
// the stream never began, so the caller can surface a real HTTP error instead.
func TestFinalizeAnthropicStreamNoMessageStart(t *testing.T) {
	state := CreateAnthropicStreamState()
	if events := FinalizeAnthropicStream(state, cif.StopReasonEndTurn); events != nil {
		t.Errorf("expected nil terminal events before message_start, got %v", events)
	}
	if events := FinalizeAnthropicStream(nil, cif.StopReasonEndTurn); events != nil {
		t.Errorf("expected nil terminal events for nil state, got %v", events)
	}
}

// TestFinalizeAnthropicStreamNoOpenBlock verifies the terminal sequence when the
// content block was already closed but message_stop never arrived.
func TestFinalizeAnthropicStreamNoOpenBlock(t *testing.T) {
	state := CreateAnthropicStreamState()
	if _, err := ConvertCIFEventToAnthropicSSE(cif.CIFStreamStart{
		Type: "stream_start", ID: "msg_1", Model: "m",
	}, state); err != nil {
		t.Fatalf("stream start: %v", err)
	}

	events := FinalizeAnthropicStream(state, cif.StopReasonEndTurn)
	if len(events) != 2 {
		t.Fatalf("expected 2 terminal events, got %d: %v", len(events), events)
	}
	if et, _ := events[0]["type"].(string); et != "message_delta" {
		t.Errorf("events[0] = %q, want message_delta", et)
	}
	if et, _ := events[1]["type"].(string); et != "message_stop" {
		t.Errorf("events[1] = %q, want message_stop", et)
	}
}
