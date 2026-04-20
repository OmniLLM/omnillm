package google

import (
	"io"
	"strings"
	"testing"

	"omnillm/internal/cif"
)

func sseBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

func collectGemini(body io.ReadCloser) []cif.CIFStreamEvent {
	ch := make(chan cif.CIFStreamEvent, 64)
	go ParseGeminiSSE(body, ch)
	var events []cif.CIFStreamEvent
	for evt := range ch {
		events = append(events, evt)
	}
	return events
}

func TestParseGeminiSSE_EmitsTextAndToolCallEvents(t *testing.T) {
	stream := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"pong"},{"functionCall":{"name":"Read","args":{"file":"README.md"}}}]},"finishReason":"FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}`

	events := collectGemini(sseBody(stream))
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	if _, ok := events[0].(cif.CIFStreamStart); !ok {
		t.Fatalf("expected stream start, got %#v", events[0])
	}
	textDelta, ok := events[1].(cif.CIFContentDelta)
	if !ok {
		t.Fatalf("expected text content delta, got %#v", events[1])
	}
	if text, ok := textDelta.Delta.(cif.TextDelta); !ok || text.Text != "pong" {
		t.Fatalf("unexpected text delta: %#v", textDelta)
	}
	toolDelta, ok := events[2].(cif.CIFContentDelta)
	if !ok {
		t.Fatalf("expected tool content delta, got %#v", events[2])
	}
	if tool, ok := toolDelta.ContentBlock.(cif.CIFToolCallPart); !ok || tool.ToolName != "Read" {
		t.Fatalf("unexpected tool delta: %#v", toolDelta)
	}
	end, ok := events[3].(cif.CIFStreamEnd)
	if !ok {
		t.Fatalf("expected stream end, got %#v", events[2])
	}
	if end.StopReason != cif.StopReasonToolUse {
		t.Fatalf("unexpected stop reason: %q", end.StopReason)
	}
	if end.Usage == nil || end.Usage.InputTokens != 5 || end.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %#v", end.Usage)
	}
}

func TestStopReasonMapsGeminiReasons(t *testing.T) {
	cases := map[string]cif.CIFStopReason{
		"STOP":          cif.StopReasonEndTurn,
		"MAX_TOKENS":    cif.StopReasonMaxTokens,
		"FUNCTION_CALL": cif.StopReasonToolUse,
		"SAFETY":        cif.StopReasonContentFilter,
		"UNKNOWN":       cif.StopReasonEndTurn,
	}
	for input, want := range cases {
		if got := StopReason(input); got != want {
			t.Fatalf("StopReason(%q) = %q, want %q", input, got, want)
		}
	}
}
