package serialization

import (
	"testing"

	"omnillm/internal/cif"
)

func TestSerializeToAnthropic_PreservesThinkingBlocks(t *testing.T) {
	signature := "thinking_sig_123"
	resp := &cif.CanonicalResponse{
		ID:    "msg_thinking",
		Model: "claude-3-5-sonnet-20241022",
		Content: []cif.CIFContentPart{
			cif.CIFThinkingPart{
				Type:      "thinking",
				Thinking:  "Let me calculate this carefully.",
				Signature: &signature,
			},
			cif.CIFTextPart{
				Type: "text",
				Text: "The answer is 345.",
			},
		},
		StopReason: cif.StopReasonEndTurn,
	}

	out, err := SerializeToAnthropic(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(out.Content))
	}
	if out.Content[0].Type != "thinking" {
		t.Fatalf("expected thinking block, got %q", out.Content[0].Type)
	}
	if out.Content[0].Thinking != "Let me calculate this carefully." {
		t.Fatalf("unexpected thinking payload: %#v", out.Content[0])
	}
	if out.Content[0].Signature == nil || *out.Content[0].Signature != signature {
		t.Fatalf("unexpected thinking signature: %#v", out.Content[0].Signature)
	}
	if out.Content[1].Type != "text" || out.Content[1].Text != "The answer is 345." {
		t.Fatalf("unexpected text block: %#v", out.Content[1])
	}
}

func TestSerializeToResponses_SerializesMessageAndFunctionCallOutput(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "resp_123",
		Model: "gpt-5.4-mini",
		Content: []cif.CIFContentPart{
			cif.CIFThinkingPart{Type: "thinking", Thinking: "Thinking through it."},
			cif.CIFTextPart{Type: "text", Text: "Done."},
			cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    "call_123",
				ToolName:      "get_weather",
				ToolArguments: map[string]interface{}{"location": "Boston"},
			},
		},
		StopReason: cif.StopReasonToolUse,
		Usage:      &cif.CIFUsage{InputTokens: 9, OutputTokens: 12},
	}

	out, err := SerializeToResponses(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Object != "realtime.response" {
		t.Fatalf("unexpected object: %q", out.Object)
	}
	if len(out.Output) != 2 {
		t.Fatalf("expected message + function call output items, got %d", len(out.Output))
	}
	if out.Output[0].Type != "message" || len(out.Output[0].Content) != 2 {
		t.Fatalf("unexpected message output item: %#v", out.Output[0])
	}
	if out.Output[0].Content[0].Text != "<thinking>\nThinking through it.\n</thinking>" {
		t.Fatalf("unexpected thinking text wrapper: %#v", out.Output[0].Content[0])
	}
	if out.Output[1].Type != "function_call" || out.Output[1].Name != "get_weather" {
		t.Fatalf("unexpected function_call output item: %#v", out.Output[1])
	}
	if out.Output[1].Arguments != `{"location":"Boston"}` {
		t.Fatalf("unexpected function call arguments: %q", out.Output[1].Arguments)
	}
	if out.Usage == nil || out.Usage.InputTokens != 9 || out.Usage.OutputTokens != 12 || out.Usage.TotalTokens != 21 {
		t.Fatalf("unexpected usage payload: %#v", out.Usage)
	}
}

func TestConvertCIFEventToResponsesSSE_IncludesTotalTokensInCompletedUsage(t *testing.T) {
	state := CreateResponsesStreamState()

	events, err := ConvertCIFEventToResponsesSSE(cif.CIFStreamEnd{
		Type: "stream_end",
		Usage: &cif.CIFUsage{
			InputTokens:  7,
			OutputTokens: 3,
		},
	}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	response, ok := events[0]["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected response payload: %#v", events[0])
	}
	usage, ok := response["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected usage payload, got %#v", response)
	}
	if usage["input_tokens"] != 7 || usage["output_tokens"] != 3 || usage["total_tokens"] != 10 {
		t.Fatalf("unexpected usage payload: %#v", usage)
	}
}

func TestConvertCIFEventToResponsesSSE_EmitsMessageLifecycle(t *testing.T) {
	state := CreateResponsesStreamState()

	startEvents, err := ConvertCIFEventToResponsesSSE(cif.CIFStreamStart{
		Type:  "stream_start",
		ID:    "resp_stream",
		Model: "gpt-5.4-mini",
	}, state)
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	deltaEvents, err := ConvertCIFEventToResponsesSSE(cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        0,
		ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
		Delta:        cif.TextDelta{Type: "text_delta", Text: "pong"},
	}, state)
	if err != nil {
		t.Fatalf("unexpected delta error: %v", err)
	}
	stopEvents, err := ConvertCIFEventToResponsesSSE(cif.CIFContentBlockStop{Type: "content_block_stop"}, state)
	if err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if len(startEvents) != 1 {
		t.Fatalf("expected 1 start event, got %d", len(startEvents))
	}
	if len(deltaEvents) != 3 {
		t.Fatalf("expected 3 delta events, got %d", len(deltaEvents))
	}
	if deltaEvents[0]["type"] != "response.output_item.added" {
		t.Fatalf("unexpected first delta event: %#v", deltaEvents[0])
	}
	if deltaEvents[1]["type"] != "response.content_block.added" {
		t.Fatalf("unexpected second delta event: %#v", deltaEvents[1])
	}
	if deltaEvents[2]["type"] != "response.output_text.delta" {
		t.Fatalf("unexpected third delta event: %#v", deltaEvents[2])
	}
	if len(stopEvents) != 2 {
		t.Fatalf("expected 2 stop events, got %d", len(stopEvents))
	}
	if stopEvents[0]["type"] != "response.output_text.done" {
		t.Fatalf("unexpected stop event: %#v", stopEvents[0])
	}
	if stopEvents[1]["type"] != "response.output_item.done" {
		t.Fatalf("unexpected final stop event: %#v", stopEvents[1])
	}
}

func TestConvertCIFEventToAnthropicSSE_ToolUseLifecycle(t *testing.T) {
	cacheRead := 3
	cacheWrite := 7
	state := CreateAnthropicStreamState()

	startEvents, err := ConvertCIFEventToAnthropicSSE(cif.CIFStreamStart{
		Type:  "stream_start",
		ID:    "stream_123",
		Model: "claude-3-5-sonnet-20241022",
	}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deltaEvents, err := ConvertCIFEventToAnthropicSSE(cif.CIFContentDelta{
		Type:  "content_delta",
		Index: 1,
		ContentBlock: cif.CIFToolCallPart{
			Type:       "tool_call",
			ToolCallID: "call_123",
			ToolName:   "shell_command",
		},
		Delta: cif.ToolArgumentsDelta{
			Type:        "tool_arguments_delta",
			PartialJSON: `{"command":"rg --files"}`,
		},
	}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	endEvents, err := ConvertCIFEventToAnthropicSSE(cif.CIFStreamEnd{
		Type:       "stream_end",
		StopReason: cif.StopReasonToolUse,
		Usage: &cif.CIFUsage{
			OutputTokens:          5,
			CacheReadInputTokens:  &cacheRead,
			CacheWriteInputTokens: &cacheWrite,
		},
	}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := append(append(startEvents, deltaEvents...), endEvents...)
	if len(events) != 6 {
		t.Fatalf("expected 6 anthropic SSE events, got %d", len(events))
	}
	if events[0]["type"] != "message_start" {
		t.Fatalf("unexpected first event: %#v", events[0])
	}
	if events[1]["type"] != "content_block_start" {
		t.Fatalf("unexpected content block start: %#v", events[1])
	}
	contentBlock := events[1]["content_block"].(map[string]interface{})
	if contentBlock["type"] != "tool_use" || contentBlock["id"] != "call_123" {
		t.Fatalf("unexpected anthropic tool block: %#v", contentBlock)
	}

	delta := events[2]["delta"].(map[string]interface{})
	if delta["type"] != "input_json_delta" || delta["partial_json"] != `{"command":"rg --files"}` {
		t.Fatalf("unexpected anthropic delta: %#v", delta)
	}

	messageDelta := events[4]["delta"].(map[string]interface{})
	stopReason, ok := messageDelta["stop_reason"].(*string)
	if !ok || stopReason == nil || *stopReason != "tool_use" {
		t.Fatalf("unexpected stop reason payload: %#v", messageDelta["stop_reason"])
	}
	usage := events[4]["usage"].(map[string]interface{})
	if usage["output_tokens"] != 5 || usage["cache_creation_input_tokens"] != 7 || usage["cache_read_input_tokens"] != 3 {
		t.Fatalf("unexpected anthropic usage payload: %#v", usage)
	}
	if events[5]["type"] != "message_stop" {
		t.Fatalf("unexpected final event: %#v", events[5])
	}
}

// TestConvertCIFEventToAnthropicSSE_SuppressThinkingBlocks verifies that when
// SuppressThinkingBlocks is true, thinking blocks are silently dropped from the
// output stream and tool_use blocks that follow are still emitted correctly.
// This is the fix for qwen3.6-plus (and other reasoning models) emitting
// reasoning_content alongside tool calls: without suppression, Claude Code's
// Anthropic SDK silently stops processing the stream and never executes the tool.
func TestConvertCIFEventToAnthropicSSE_SuppressThinkingBlocks(t *testing.T) {
	state := CreateAnthropicStreamState()
	state.SuppressThinkingBlocks = true

	feed := func(event cif.CIFStreamEvent) []map[string]interface{} {
		evts, err := ConvertCIFEventToAnthropicSSE(event, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return evts
	}

	var allEvents []map[string]interface{}

	// stream_start
	allEvents = append(allEvents, feed(cif.CIFStreamStart{ID: "msg_1", Model: "qwen3.6-plus"})...)

	// thinking block start (index -1, provider sentinel)
	allEvents = append(allEvents, feed(cif.CIFContentDelta{
		Index:        -1,
		ContentBlock: cif.CIFThinkingPart{Type: "thinking", Thinking: ""},
		Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: "Let me explore..."},
	})...)

	// thinking deltas (same index, no ContentBlock)
	allEvents = append(allEvents, feed(cif.CIFContentDelta{
		Index: -1,
		Delta: cif.ThinkingDelta{Type: "thinking_delta", Thinking: " deeper."},
	})...)

	// tool_use block (index 1)
	allEvents = append(allEvents, feed(cif.CIFContentDelta{
		Index: 1,
		ContentBlock: cif.CIFToolCallPart{
			Type:       "tool_call",
			ToolCallID: "call_agent_1",
			ToolName:   "Agent",
		},
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"description":"explore","prompt":"..."}`},
	})...)

	// stream end
	allEvents = append(allEvents, feed(cif.CIFStreamEnd{StopReason: cif.StopReasonToolUse})...)

	// Verify: no thinking-related events in output
	for _, evt := range allEvents {
		if cb, ok := evt["content_block"].(map[string]interface{}); ok {
			if cb["type"] == "thinking" {
				t.Errorf("expected thinking block to be suppressed, got event: %#v", evt)
			}
		}
		if delta, ok := evt["delta"].(map[string]interface{}); ok {
			if delta["type"] == "thinking_delta" {
				t.Errorf("expected thinking delta to be suppressed, got event: %#v", evt)
			}
		}
	}

	// Verify: tool_use block is present
	var foundToolUse bool
	for _, evt := range allEvents {
		if cb, ok := evt["content_block"].(map[string]interface{}); ok {
			if cb["type"] == "tool_use" && cb["id"] == "call_agent_1" {
				foundToolUse = true
			}
		}
	}
	if !foundToolUse {
		t.Errorf("expected tool_use block to be present after suppressed thinking blocks; events: %#v", allEvents)
	}

	// Verify tool_use is at index 0 (first non-suppressed block)
	for _, evt := range allEvents {
		if evt["type"] == "content_block_start" {
			if cb, ok := evt["content_block"].(map[string]interface{}); ok && cb["type"] == "tool_use" {
				if evt["index"] != 0 {
					t.Errorf("expected suppressed thinking to not consume a block index; tool_use got index %v, want 0", evt["index"])
				}
			}
		}
	}
}

// TestConvertCIFEventToAnthropicSSE_ThinkingBlocksPassedThroughWhenNotSuppressed
// verifies that thinking blocks ARE forwarded when SuppressThinkingBlocks is false
// (the default) — i.e. for clients that opted in to interleaved-thinking.
func TestConvertCIFEventToAnthropicSSE_ThinkingBlocksPassedThroughWhenNotSuppressed(t *testing.T) {
	state := CreateAnthropicStreamState()
	// SuppressThinkingBlocks is false by default

	feed := func(event cif.CIFStreamEvent) []map[string]interface{} {
		evts, err := ConvertCIFEventToAnthropicSSE(event, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return evts
	}

	var allEvents []map[string]interface{}
	allEvents = append(allEvents, feed(cif.CIFStreamStart{ID: "msg_2", Model: "qwen3.6-plus"})...)
	allEvents = append(allEvents, feed(cif.CIFContentDelta{
		Index:        -1,
		ContentBlock: cif.CIFThinkingPart{Type: "thinking", Thinking: ""},
		Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: "thinking text"},
	})...)
	allEvents = append(allEvents, feed(cif.CIFStreamEnd{StopReason: cif.StopReasonEndTurn})...)

	var foundThinking bool
	for _, evt := range allEvents {
		if cb, ok := evt["content_block"].(map[string]interface{}); ok {
			if cb["type"] == "thinking" {
				foundThinking = true
			}
		}
	}
	if !foundThinking {
		t.Error("expected thinking block to be present when SuppressThinkingBlocks=false")
	}
}

func TestConvertCIFEventToResponsesSSE_TextLifecycle(t *testing.T) {
	state := CreateResponsesStreamState()

	startEvents, err := ConvertCIFEventToResponsesSSE(cif.CIFStreamStart{
		Type:  "stream_start",
		ID:    "resp_456",
		Model: "gpt-5.4-mini",
	}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deltaEvents, err := ConvertCIFEventToResponsesSSE(cif.CIFContentDelta{
		Type:  "content_delta",
		Index: 0,
		ContentBlock: cif.CIFTextPart{
			Type: "text",
			Text: "",
		},
		Delta: cif.TextDelta{
			Type: "text_delta",
			Text: "Hello there",
		},
	}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	endEvents, err := ConvertCIFEventToResponsesSSE(cif.CIFStreamEnd{
		Type:       "stream_end",
		StopReason: cif.StopReasonEndTurn,
		Usage:      &cif.CIFUsage{InputTokens: 4, OutputTokens: 2},
	}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := append(append(startEvents, deltaEvents...), endEvents...)
	if len(events) != 7 {
		t.Fatalf("expected 7 responses SSE events, got %d", len(events))
	}
	if events[0]["type"] != "response.created" {
		t.Fatalf("unexpected first event: %#v", events[0])
	}
	if events[1]["type"] != "response.output_item.added" {
		t.Fatalf("unexpected item.added event: %#v", events[1])
	}
	if events[2]["type"] != "response.content_block.added" {
		t.Fatalf("unexpected content_block.added event: %#v", events[2])
	}
	if events[3]["type"] != "response.output_text.delta" || events[3]["delta"] != "Hello there" {
		t.Fatalf("unexpected output_text.delta event: %#v", events[3])
	}
	if events[4]["type"] != "response.output_text.done" || events[4]["text"] != "Hello there" {
		t.Fatalf("unexpected output_text.done event: %#v", events[4])
	}
	if events[5]["type"] != "response.output_item.done" {
		t.Fatalf("unexpected output_item.done event: %#v", events[5])
	}
	completed := events[6]["response"].(map[string]interface{})
	if events[6]["type"] != "response.completed" {
		t.Fatalf("unexpected response.completed event: %#v", events[6])
	}
	usage := completed["usage"].(map[string]interface{})
	if usage["input_tokens"] != 4 || usage["output_tokens"] != 2 {
		t.Fatalf("unexpected responses usage payload: %#v", usage)
	}
}
