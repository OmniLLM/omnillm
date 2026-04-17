package generic

import (
	"io"
	"strings"
	"testing"

	"omnimodel/internal/cif"
)

func collectAnthropicSSEEvents(t *testing.T, sse string) []cif.CIFStreamEvent {
	t.Helper()

	eventCh := make(chan cif.CIFStreamEvent, 16)
	go parseAnthropicSSE(io.NopCloser(strings.NewReader(sse)), eventCh)

	var events []cif.CIFStreamEvent
	for event := range eventCh {
		events = append(events, event)
	}
	return events
}

func finalStreamEndEvent(t *testing.T, events []cif.CIFStreamEvent) cif.CIFStreamEnd {
	t.Helper()

	for i := len(events) - 1; i >= 0; i-- {
		if end, ok := events[i].(cif.CIFStreamEnd); ok {
			return end
		}
	}

	t.Fatalf("no CIFStreamEnd found in events: %#v", events)
	return cif.CIFStreamEnd{}
}

func TestParseAnthropicSSEPreservesMessageStartUsageWhenDeltaOmitsUsage(t *testing.T) {
	sse := strings.Join([]string{
		"event: message_start",
		"data: {\"message\":{\"id\":\"msg_1\",\"model\":\"qwen3.6-plus\",\"usage\":{\"input_tokens\":12,\"cache_creation_input_tokens\":3}}}",
		"",
		"event: message_delta",
		"data: {\"delta\":{\"stop_reason\":\"end_turn\"}}",
		"",
		"event: message_stop",
		"data: {}",
		"",
	}, "\n")

	events := collectAnthropicSSEEvents(t, sse)
	end := finalStreamEndEvent(t, events)

	if end.Usage == nil {
		t.Fatalf("expected usage, got nil")
	}
	if end.Usage.InputTokens != 12 {
		t.Fatalf("input_tokens = %d, want 12", end.Usage.InputTokens)
	}
	if end.Usage.OutputTokens != 0 {
		t.Fatalf("output_tokens = %d, want 0", end.Usage.OutputTokens)
	}
	if end.Usage.CacheWriteInputTokens == nil || *end.Usage.CacheWriteInputTokens != 3 {
		t.Fatalf("cache_creation_input_tokens = %#v, want 3", end.Usage.CacheWriteInputTokens)
	}
}

func TestParseAnthropicSSEUsesMessageDeltaUsageWhenPresent(t *testing.T) {
	sse := strings.Join([]string{
		"event: message_start",
		"data: {\"message\":{\"id\":\"msg_2\",\"model\":\"qwen3.6-plus\",\"usage\":{\"input_tokens\":7}}}",
		"",
		"event: message_delta",
		"data: {\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":9,\"cache_read_input_tokens\":4}}",
		"",
		"event: message_stop",
		"data: {}",
		"",
	}, "\n")

	events := collectAnthropicSSEEvents(t, sse)
	end := finalStreamEndEvent(t, events)

	if end.Usage == nil {
		t.Fatalf("expected usage, got nil")
	}
	if end.Usage.InputTokens != 7 {
		t.Fatalf("input_tokens = %d, want 7", end.Usage.InputTokens)
	}
	if end.Usage.OutputTokens != 9 {
		t.Fatalf("output_tokens = %d, want 9", end.Usage.OutputTokens)
	}
	if end.Usage.CacheReadInputTokens == nil || *end.Usage.CacheReadInputTokens != 4 {
		t.Fatalf("cache_read_input_tokens = %#v, want 4", end.Usage.CacheReadInputTokens)
	}
	if end.StopReason != cif.StopReasonToolUse {
		t.Fatalf("stop_reason = %q, want %q", end.StopReason, cif.StopReasonToolUse)
	}
}

func TestParseAnthropicSSEReturnsNilUsageWhenAbsentEverywhere(t *testing.T) {
	sse := strings.Join([]string{
		"event: message_start",
		"data: {\"message\":{\"id\":\"msg_3\",\"model\":\"qwen3.6-plus\"}}",
		"",
		"event: message_delta",
		"data: {\"delta\":{\"stop_reason\":\"end_turn\"}}",
		"",
		"event: message_stop",
		"data: {}",
		"",
	}, "\n")

	events := collectAnthropicSSEEvents(t, sse)
	end := finalStreamEndEvent(t, events)

	if end.Usage != nil {
		t.Fatalf("expected nil usage, got %#v", end.Usage)
	}
}
