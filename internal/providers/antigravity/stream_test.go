package antigravity

import (
	"io"
	"strings"
	"testing"

	"omnillm/internal/cif"
)

func sseBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

func collectAntigravity(body io.ReadCloser) []cif.CIFStreamEvent {
	ch := make(chan cif.CIFStreamEvent, 64)
	go ParseAntigravitySSE(body, ch)
	var events []cif.CIFStreamEvent
	for evt := range ch {
		events = append(events, evt)
	}
	return events
}

func TestParseAntigravitySSE_EmitsTextToolAndEnd(t *testing.T) {
	stream := `data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"pong"},{"functionCall":{"name":"Read","args":{"file":"README.md"}}}]},"finishReason":"FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2}}}`

	events := collectAntigravity(sseBody(stream))
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	if _, ok := events[0].(cif.CIFStreamStart); !ok {
		t.Fatalf("expected stream start, got %#v", events[0])
	}
	if delta, ok := events[1].(cif.CIFContentDelta); !ok {
		t.Fatalf("expected text delta, got %#v", events[1])
	} else if text, ok := delta.Delta.(cif.TextDelta); !ok || text.Text != "pong" {
		t.Fatalf("unexpected text delta: %#v", delta)
	}
	if delta, ok := events[2].(cif.CIFContentDelta); !ok {
		t.Fatalf("expected tool delta, got %#v", events[2])
	} else if tool, ok := delta.ContentBlock.(cif.CIFToolCallPart); !ok || tool.ToolName != "Read" {
		t.Fatalf("unexpected tool delta: %#v", delta)
	}
	if end, ok := events[3].(cif.CIFStreamEnd); !ok || end.StopReason != cif.StopReasonToolUse {
		t.Fatalf("unexpected end event: %#v", events[3])
	}
}

func TestStopReasonMapsAntigravityReasons(t *testing.T) {
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
