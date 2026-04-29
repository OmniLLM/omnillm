package serialization

import (
	"omnillm/internal/cif"
	"testing"
)

// ─── SerializeToOpenAI ───

func TestSerializeToOpenAI_TextContent(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "resp_001",
		Model: "gpt-4o",
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "Hello there!"},
		},
		StopReason: cif.StopReasonEndTurn,
	}

	out, err := SerializeToOpenAI(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "resp_001" {
		t.Errorf("expected id=resp_001, got %q", out.ID)
	}
	if out.Model != "gpt-4o" {
		t.Errorf("expected model=gpt-4o, got %q", out.Model)
	}
	if out.Object != "chat.completion" {
		t.Errorf("expected object=chat.completion, got %q", out.Object)
	}
	if len(out.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(out.Choices))
	}
	choice := out.Choices[0]
	if choice.Message == nil {
		t.Fatal("expected non-nil message")
	}
	if choice.Message.Content == nil || *choice.Message.Content != "Hello there!" {
		t.Errorf("unexpected message content: %v", choice.Message.Content)
	}
	if choice.Message.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", choice.Message.Role)
	}
}

func TestSerializeToOpenAI_StopReasonMapping(t *testing.T) {
	cases := []struct {
		reason   cif.CIFStopReason
		expected string
	}{
		{cif.StopReasonEndTurn, "stop"},
		{cif.StopReasonMaxTokens, "length"},
		{cif.StopReasonToolUse, "tool_calls"},
		{cif.StopReasonContentFilter, "content_filter"},
		{cif.StopReasonStopSequence, "stop"},
	}

	for _, c := range cases {
		resp := &cif.CanonicalResponse{
			ID:         "r1",
			Model:      "gpt-4o",
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ok"}},
			StopReason: c.reason,
		}
		out, err := SerializeToOpenAI(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Choices[0].FinishReason == nil || *out.Choices[0].FinishReason != c.expected {
			t.Errorf("stop reason %q: expected %q, got %v", c.reason, c.expected, out.Choices[0].FinishReason)
		}
	}
}

func TestSerializeToOpenAI_ToolCall(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "resp_002",
		Model: "gpt-4o",
		Content: []cif.CIFContentPart{
			cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    "call_xyz",
				ToolName:      "get_weather",
				ToolArguments: map[string]interface{}{"location": "NYC"},
			},
		},
		StopReason: cif.StopReasonToolUse,
	}

	out, err := SerializeToOpenAI(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(out.Choices[0].Message.ToolCalls))
	}
	if out.Choices[0].Message.Content != nil {
		t.Fatalf("expected content to be nil for tool-call-only assistant message, got %v", out.Choices[0].Message.Content)
	}
	tc := out.Choices[0].Message.ToolCalls[0]
	if tc.ID != "call_xyz" {
		t.Errorf("expected id=call_xyz, got %q", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected get_weather, got %q", tc.Function.Name)
	}
	if tc.Type != "function" {
		t.Errorf("expected type=function, got %q", tc.Type)
	}
}

func TestSerializeToOpenAI_UsageFields(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "resp_003",
		Model: "gpt-4o",
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "ok"},
		},
		StopReason: cif.StopReasonEndTurn,
		Usage:      &cif.CIFUsage{InputTokens: 10, OutputTokens: 5},
	}

	out, err := SerializeToOpenAI(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if out.Usage.PromptTokens != 10 {
		t.Errorf("expected prompt_tokens=10, got %d", out.Usage.PromptTokens)
	}
	if out.Usage.CompletionTokens != 5 {
		t.Errorf("expected completion_tokens=5, got %d", out.Usage.CompletionTokens)
	}
	if out.Usage.TotalTokens != 15 {
		t.Errorf("expected total_tokens=15, got %d", out.Usage.TotalTokens)
	}
}

func TestSerializeToOpenAI_MultipleTextParts(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "resp_004",
		Model: "gpt-4o",
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "Hello "},
			cif.CIFTextPart{Type: "text", Text: "world!"},
		},
		StopReason: cif.StopReasonEndTurn,
	}

	out, err := SerializeToOpenAI(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Choices[0].Message.Content == nil || *out.Choices[0].Message.Content != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %v", out.Choices[0].Message.Content)
	}
}

// ─── SerializeToAnthropic ───

func TestSerializeToAnthropic_TextContent(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "msg_001",
		Model: "claude-3-5-sonnet-20241022",
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "I can help!"},
		},
		StopReason: cif.StopReasonEndTurn,
	}

	out, err := SerializeToAnthropic(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "msg_001" {
		t.Errorf("expected id=msg_001, got %q", out.ID)
	}
	if out.Type != "message" {
		t.Errorf("expected type=message, got %q", out.Type)
	}
	if out.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", out.Role)
	}
	if len(out.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(out.Content))
	}
	if out.Content[0].Type != "text" {
		t.Errorf("expected type=text, got %q", out.Content[0].Type)
	}
	if out.Content[0].Text != "I can help!" {
		t.Errorf("expected 'I can help!', got %q", out.Content[0].Text)
	}
}

func TestSerializeToAnthropic_StopReasonMapping(t *testing.T) {
	cases := []struct {
		reason   cif.CIFStopReason
		expected string
	}{
		{cif.StopReasonEndTurn, "end_turn"},
		{cif.StopReasonMaxTokens, "max_tokens"},
		{cif.StopReasonToolUse, "tool_use"},
		{cif.StopReasonContentFilter, "content_filter"},
		{cif.StopReasonStopSequence, "stop_sequence"},
		{cif.StopReasonError, "error"},
	}

	for _, c := range cases {
		resp := &cif.CanonicalResponse{
			ID:         "m1",
			Model:      "claude",
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ok"}},
			StopReason: c.reason,
		}
		out, err := SerializeToAnthropic(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.StopReason == nil || *out.StopReason != c.expected {
			t.Errorf("stop reason %q: expected %q, got %v", c.reason, c.expected, out.StopReason)
		}
	}
}

func TestSerializeToAnthropic_ToolUseBlock(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "msg_002",
		Model: "claude-3-5-sonnet-20241022",
		Content: []cif.CIFContentPart{
			cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    "toolu_01",
				ToolName:      "get_weather",
				ToolArguments: map[string]interface{}{"location": "NYC"},
			},
		},
		StopReason: cif.StopReasonToolUse,
	}

	out, err := SerializeToAnthropic(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(out.Content))
	}
	block := out.Content[0]
	if block.Type != "tool_use" {
		t.Errorf("expected type=tool_use, got %q", block.Type)
	}
	if block.ID != "toolu_01" {
		t.Errorf("expected id=toolu_01, got %q", block.ID)
	}
	if block.Name != "get_weather" {
		t.Errorf("expected name=get_weather, got %q", block.Name)
	}
}

func TestSerializeToAnthropic_NormalizesCopilotToolUseID(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "msg_tooluse",
		Model: "claude-haiku-4.5",
		Content: []cif.CIFContentPart{
			cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    "tooluse_abc123",
				ToolName:      "Read",
				ToolArguments: map[string]interface{}{"file_path": "README.md"},
			},
		},
		StopReason: cif.StopReasonToolUse,
	}

	out, err := SerializeToAnthropic(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Content) != 1 {
		t.Fatalf("expected one content block, got %d", len(out.Content))
	}
	if out.Content[0].ID != "toolu_abc123" {
		t.Fatalf("expected normalized tool use id, got %q", out.Content[0].ID)
	}
}

func TestSerializeToAnthropic_UsageFields(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:         "msg_003",
		Model:      "claude",
		Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ok"}},
		StopReason: cif.StopReasonEndTurn,
		Usage:      &cif.CIFUsage{InputTokens: 20, OutputTokens: 8},
	}

	out, err := SerializeToAnthropic(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if out.Usage.InputTokens != 20 {
		t.Errorf("expected input_tokens=20, got %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 8 {
		t.Errorf("expected output_tokens=8, got %d", out.Usage.OutputTokens)
	}
}

func TestSerializeToAnthropic_ThinkingBlock(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "msg_004",
		Model: "claude",
		Content: []cif.CIFContentPart{
			cif.CIFThinkingPart{Type: "thinking", Thinking: "Let me think..."},
			cif.CIFTextPart{Type: "text", Text: "Answer here."},
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
		t.Errorf("expected first block type=thinking, got %q", out.Content[0].Type)
	}
}

func TestSerializeToAnthropicWithSuppression_SkipsThinkingBlock(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "msg_005",
		Model: "claude",
		Content: []cif.CIFContentPart{
			cif.CIFThinkingPart{Type: "thinking", Thinking: "Let me think..."},
			cif.CIFTextPart{Type: "text", Text: "Answer here."},
		},
		StopReason: cif.StopReasonEndTurn,
	}

	out, err := SerializeToAnthropicWithSuppression(resp, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(out.Content))
	}
	if out.Content[0].Type != "text" {
		t.Errorf("expected first block type=text, got %q", out.Content[0].Type)
	}
}

// ─── ConvertCIFEventToOpenAISSE ───

func TestConvertCIFEventToOpenAISSE_StreamStart(t *testing.T) {
	state := CreateOpenAIStreamState()
	event := cif.CIFStreamStart{
		Type:  "stream_start",
		ID:    "chatcmpl-001",
		Model: "gpt-4o",
	}
	sseData, err := ConvertCIFEventToOpenAISSE(event, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sseData == "" {
		t.Error("expected non-empty SSE data for stream_start")
	}
	if state.Model != "gpt-4o" {
		t.Errorf("expected state.Model=gpt-4o, got %q", state.Model)
	}
}

func TestConvertCIFEventToOpenAISSE_TextDelta(t *testing.T) {
	state := CreateOpenAIStreamState()
	state.Model = "gpt-4o"
	state.ID = "chatcmpl-001"
	event := cif.CIFContentDelta{
		Type:  "content_delta",
		Delta: cif.TextDelta{Text: "Hello"},
	}
	sseData, err := ConvertCIFEventToOpenAISSE(event, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sseData == "" {
		t.Error("expected non-empty SSE data for text delta")
	}
}

func TestConvertCIFEventToOpenAISSE_StreamEnd(t *testing.T) {
	state := CreateOpenAIStreamState()
	state.Model = "gpt-4o"
	state.ID = "chatcmpl-001"
	event := cif.CIFStreamEnd{
		Type:       "stream_end",
		StopReason: cif.StopReasonEndTurn,
	}
	sseData, err := ConvertCIFEventToOpenAISSE(event, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain [DONE] marker
	if len(sseData) == 0 {
		t.Error("expected non-empty SSE data for stream_end")
	}
}
