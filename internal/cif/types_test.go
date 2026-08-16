package cif

import (
	"encoding/json"
	"testing"
)

// ─── Content part GetType ───

func TestCIFTextPart_GetType(t *testing.T) {
	p := CIFTextPart{Type: "text", Text: "hello"}
	if p.GetType() != "text" {
		t.Errorf("expected 'text', got %q", p.GetType())
	}
}

func TestCIFImagePart_GetType(t *testing.T) {
	url := "https://example.com/img.png"
	p := CIFImagePart{Type: "image", URL: &url}
	if p.GetType() != "image" {
		t.Errorf("expected 'image', got %q", p.GetType())
	}
}

func TestCIFThinkingPart_GetType(t *testing.T) {
	p := CIFThinkingPart{Type: "thinking", Thinking: "hmm"}
	if p.GetType() != "thinking" {
		t.Errorf("expected 'thinking', got %q", p.GetType())
	}
}

func TestCIFToolCallPart_GetType(t *testing.T) {
	p := CIFToolCallPart{Type: "tool_call", ToolName: "get_weather"}
	if p.GetType() != "tool_call" {
		t.Errorf("expected 'tool_call', got %q", p.GetType())
	}
}

func TestCIFToolResultPart_GetType(t *testing.T) {
	p := CIFToolResultPart{Type: "tool_result", ToolCallID: "call_1"}
	if p.GetType() != "tool_result" {
		t.Errorf("expected 'tool_result', got %q", p.GetType())
	}
}

// ─── Message GetRole ───

func TestCIFSystemMessage_GetRole(t *testing.T) {
	m := CIFSystemMessage{Role: "system", Content: SystemBlocksFromText("You are helpful.")}
	if m.GetRole() != "system" {
		t.Errorf("expected 'system', got %q", m.GetRole())
	}
}

func TestCIFUserMessage_GetRole(t *testing.T) {
	m := CIFUserMessage{Role: "user"}
	if m.GetRole() != "user" {
		t.Errorf("expected 'user', got %q", m.GetRole())
	}
}

func TestCIFAssistantMessage_GetRole(t *testing.T) {
	m := CIFAssistantMessage{Role: "assistant"}
	if m.GetRole() != "assistant" {
		t.Errorf("expected 'assistant', got %q", m.GetRole())
	}
}

// ─── Stream event GetEventType ───

func TestCIFStreamStart_GetEventType(t *testing.T) {
	e := CIFStreamStart{Type: "stream_start"}
	if e.GetEventType() != "stream_start" {
		t.Errorf("expected 'stream_start', got %q", e.GetEventType())
	}
}

func TestCIFContentDelta_GetEventType(t *testing.T) {
	e := CIFContentDelta{Type: "content_delta"}
	if e.GetEventType() != "content_delta" {
		t.Errorf("expected 'content_delta', got %q", e.GetEventType())
	}
}

func TestCIFContentBlockStop_GetEventType(t *testing.T) {
	e := CIFContentBlockStop{Type: "content_block_stop"}
	if e.GetEventType() != "content_block_stop" {
		t.Errorf("expected 'content_block_stop', got %q", e.GetEventType())
	}
}

func TestCIFStreamEnd_GetEventType(t *testing.T) {
	e := CIFStreamEnd{Type: "stream_end"}
	if e.GetEventType() != "stream_end" {
		t.Errorf("expected 'stream_end', got %q", e.GetEventType())
	}
}

func TestCIFStreamError_GetEventType(t *testing.T) {
	e := CIFStreamError{Type: "stream_error"}
	if e.GetEventType() != "stream_error" {
		t.Errorf("expected 'stream_error', got %q", e.GetEventType())
	}
}

// ─── Stop reason constants ───

func TestStopReasonConstants(t *testing.T) {
	cases := []struct {
		reason   CIFStopReason
		expected string
	}{
		{StopReasonEndTurn, "end_turn"},
		{StopReasonMaxTokens, "max_tokens"},
		{StopReasonToolUse, "tool_use"},
		{StopReasonStopSequence, "stop_sequence"},
		{StopReasonContentFilter, "content_filter"},
		{StopReasonError, "error"},
	}
	for _, c := range cases {
		if string(c.reason) != c.expected {
			t.Errorf("StopReason %q: expected %q", c.reason, c.expected)
		}
	}
}

// ─── MarshalCIFContentPart ───

func TestMarshalCIFContentPart_Text(t *testing.T) {
	p := CIFTextPart{Type: "text", Text: "hello world"}
	b, err := MarshalCIFContentPart(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["type"] != "text" {
		t.Errorf("expected type=text, got %v", m["type"])
	}
	if m["text"] != "hello world" {
		t.Errorf("expected text='hello world', got %v", m["text"])
	}
}

func TestMarshalCIFContentPart_ToolCall(t *testing.T) {
	p := CIFToolCallPart{
		Type:          "tool_call",
		ToolCallID:    "call_abc",
		ToolName:      "get_weather",
		ToolArguments: map[string]interface{}{"location": "NYC"},
	}
	b, err := MarshalCIFContentPart(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["type"] != "tool_call" {
		t.Errorf("expected type=tool_call, got %v", m["type"])
	}
	if m["toolName"] != "get_weather" {
		t.Errorf("expected toolName=get_weather, got %v", m["toolName"])
	}
}

func TestMarshalCIFContentPart_CustomToolCall(t *testing.T) {
	rawInput := "printf 'a b\\n'"
	part := CIFToolCallPart{
		Type:          "tool_call",
		ToolCallID:    "call_exec",
		ToolName:      "exec",
		ToolArguments: map[string]interface{}{"input": rawInput},
		ToolKind:      CIFToolKindCustom,
		RawInput:      &rawInput,
		Namespace:     "functions",
	}
	encoded, err := MarshalCIFContentPart(part)
	if err != nil {
		t.Fatalf("marshal custom tool call: %v", err)
	}
	var decoded ContentPartJSON
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode custom tool call: %v", err)
	}
	if decoded.ToolKind != CIFToolKindCustom || decoded.RawInput == nil || *decoded.RawInput != rawInput || decoded.Namespace != "functions" {
		t.Fatalf("custom tool metadata changed: %#v", decoded)
	}
}

func TestMarshalCIFContentPart_CustomToolResult(t *testing.T) {
	output := []interface{}{map[string]interface{}{"type": "input_text", "text": "ok"}}
	encoded, err := MarshalCIFContentPart(CIFToolResultPart{
		Type:         "tool_result",
		ToolCallID:   "call_exec",
		Content:      `[{"type":"input_text","text":"ok"}]`,
		ToolKind:     CIFToolKindCustom,
		CustomOutput: output,
	})
	if err != nil {
		t.Fatalf("marshal custom tool result: %v", err)
	}
	var decoded ContentPartJSON
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode custom tool result: %v", err)
	}
	if decoded.ToolKind != CIFToolKindCustom || decoded.CustomOutput == nil {
		t.Fatalf("custom output metadata changed: %#v", decoded)
	}
}

func TestMarshalCIFContentPart_Image(t *testing.T) {
	url := "https://example.com/pic.jpg"
	p := CIFImagePart{Type: "image", MediaType: "image/jpeg", URL: &url}
	b, err := MarshalCIFContentPart(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["type"] != "image" {
		t.Errorf("expected type=image, got %v", m["type"])
	}
}

// ─── CanonicalRequest structure ───

func TestCanonicalRequest_Fields(t *testing.T) {
	temp := 0.7
	maxTok := 1024
	req := &CanonicalRequest{
		Model:       "gpt-4o",
		Temperature: &temp,
		MaxTokens:   &maxTok,
		Stream:      true,
		Messages: []CIFMessage{
			CIFSystemMessage{Role: "system", Content: SystemBlocksFromText("Be helpful")},
			CIFUserMessage{Role: "user", Content: []CIFContentPart{
				CIFTextPart{Type: "text", Text: "Hello"},
			}},
		},
	}
	if req.Model != "gpt-4o" {
		t.Errorf("unexpected model: %q", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].GetRole() != "system" {
		t.Errorf("first message should be system")
	}
	if req.Messages[1].GetRole() != "user" {
		t.Errorf("second message should be user")
	}
}

func TestPromptCacheValidationAndUsage(t *testing.T) {
	ttl := CIFCacheTTL1h
	control := &CIFCacheControl{Type: "ephemeral", TTL: &ttl}
	req := &CanonicalRequest{
		Model:  "claude-opus-5",
		System: []CIFSystemBlock{{Type: "text", Text: "stable", CacheControl: control}},
		Messages: []CIFMessage{
			CIFUserMessage{Role: "user", Content: []CIFContentPart{CIFTextPart{Type: "text", Text: "varying"}}},
		},
	}
	if err := ValidatePromptCache(req); err != nil {
		t.Fatalf("valid prompt cache rejected: %v", err)
	}

	read := 40
	usage := UsageFromTotal(100, 5, &read)
	if usage.UncachedInputTokens == nil || *usage.UncachedInputTokens != 60 || usage.AnthropicUncachedInput() != 60 {
		t.Fatalf("unexpected total usage normalization: %#v", usage)
	}

	write5m, write1h := 20, 10
	exclusive, err := UsageFromExclusiveBuckets(30, 40, 30, 5, &write5m, &write1h)
	if err != nil {
		t.Fatalf("exclusive usage: %v", err)
	}
	if exclusive.InputTokens != 100 || exclusive.AnthropicUncachedInput() != 30 {
		t.Fatalf("unexpected exclusive usage: %#v", exclusive)
	}
}

func TestUsageFromTotalIgnoresInvalidCachedSubset(t *testing.T) {
	for _, cached := range []int{-1, 101} {
		usage := UsageFromTotal(100, 5, &cached)
		if usage.CacheReadInputTokens != nil || usage.UncachedInputTokens != nil {
			t.Fatalf("invalid cached subset was retained: %#v", usage)
		}
	}
}

func TestPromptCacheRejectsFifthBreakpoint(t *testing.T) {
	control := &CIFCacheControl{Type: "ephemeral"}
	req := &CanonicalRequest{Model: "claude-opus-5"}
	for i := 0; i < MaxPromptCacheBreakpoints+1; i++ {
		req.System = append(req.System, CIFSystemBlock{Type: "text", Text: "x", CacheControl: control})
	}
	if err := ValidatePromptCache(req); err == nil {
		t.Fatal("expected fifth breakpoint to fail validation")
	}
}

func TestMarshalCIFContentPartPreservesCacheControl(t *testing.T) {
	ttl := CIFCacheTTL5m
	encoded, err := MarshalCIFContentPart(CIFTextPart{
		Type: "text", Text: "stable",
		CacheControl: &CIFCacheControl{Type: "ephemeral", TTL: &ttl},
	})
	if err != nil {
		t.Fatalf("marshal cache control: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode cache control: %v", err)
	}
	cache, ok := payload["cacheControl"].(map[string]interface{})
	if !ok || cache["type"] != "ephemeral" || cache["ttl"] != "5m" {
		t.Fatalf("cache control changed: %#v", payload)
	}
}

// ─── CanonicalResponse structure ───

func TestCanonicalResponse_Fields(t *testing.T) {
	resp := &CanonicalResponse{
		ID:         "msg_123",
		Model:      "claude-sonnet-4",
		StopReason: StopReasonEndTurn,
		Content: []CIFContentPart{
			CIFTextPart{Type: "text", Text: "Sure!"},
		},
		Usage: &CIFUsage{InputTokens: 10, OutputTokens: 5},
	}
	if resp.ID != "msg_123" {
		t.Errorf("unexpected ID: %q", resp.ID)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Errorf("unexpected stop reason: %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("unexpected input tokens: %d", resp.Usage.InputTokens)
	}
}
