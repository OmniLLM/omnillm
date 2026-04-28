package shared

import (
	"omnillm/internal/cif"
	"strings"
	"testing"
)

func TestCIFMessagesToOpenAI_ConvertsMixedMessageTypes(t *testing.T) {
	toolErr := true
	messages := []cif.CIFMessage{
		cif.CIFSystemMessage{Role: "system", Content: "Be terse."},
		cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "Hello"},
		}},
		cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
			cif.CIFToolResultPart{Type: "tool_result", ToolCallID: "call_1", Content: "Sunny", IsError: &toolErr},
		}},
		cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{
			cif.CIFThinkingPart{Type: "thinking", Thinking: "Let me think."},
			cif.CIFTextPart{Type: "text", Text: "Done."},
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_2", ToolName: "get_weather", ToolArguments: map[string]interface{}{"location": "Boston"}},
		}},
	}

	out := CIFMessagesToOpenAI(messages)
	if len(out) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(out))
	}
	if out[0]["role"] != "system" || out[0]["content"] != "Be terse." {
		t.Fatalf("unexpected system message: %#v", out[0])
	}
	if out[1]["role"] != "user" || out[1]["content"] != "Hello" {
		t.Fatalf("unexpected user message: %#v", out[1])
	}
	if out[2]["role"] != "tool" || out[2]["tool_call_id"] != "call_1" {
		t.Fatalf("unexpected tool result message: %#v", out[2])
	}
	assistant := out[3]
	if assistant["role"] != "assistant" {
		t.Fatalf("unexpected assistant message: %#v", assistant)
	}
	if assistant["content"] != "Done." {
		t.Fatalf("unexpected assistant content: %#v", assistant)
	}
	if assistant["reasoning_content"] != "Let me think." {
		t.Fatalf("unexpected reasoning content: %#v", assistant)
	}
	toolCalls, ok := assistant["tool_calls"].([]map[string]interface{})
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("unexpected tool calls: %#v", assistant["tool_calls"])
	}
}

func TestOpenAIRespToCIF_ConvertsTextToolCallsAndUsage(t *testing.T) {
	resp := map[string]interface{}{
		"id":    "chatcmpl_123",
		"model": "gpt-4o",
		"choices": []interface{}{
			map[string]interface{}{
				"finish_reason": "tool_calls",
				"message": map[string]interface{}{
					"content": "Done.",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_1",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"location":"Boston"}`,
							},
						},
					},
				},
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(11),
			"completion_tokens": float64(2),
		},
	}

	out := OpenAIRespToCIF(resp)
	if out.ID != "chatcmpl_123" || out.Model != "gpt-4o" {
		t.Fatalf("unexpected response identity: %#v", out)
	}
	if out.StopReason != cif.StopReasonToolUse {
		t.Fatalf("unexpected stop reason: %q", out.StopReason)
	}
	if len(out.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(out.Content))
	}
	if text, ok := out.Content[0].(cif.CIFTextPart); !ok || text.Text != "Done." {
		t.Fatalf("unexpected text part: %#v", out.Content[0])
	}
	if tool, ok := out.Content[1].(cif.CIFToolCallPart); !ok || tool.ToolCallID != "call_1" || tool.ToolName != "get_weather" {
		t.Fatalf("unexpected tool call part: %#v", out.Content[1])
	}
	if out.Usage == nil || out.Usage.InputTokens != 11 || out.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %#v", out.Usage)
	}
}

func TestNormalizeOpenAICompatibleAPIFormat(t *testing.T) {
	cases := map[string]string{
		"responses":        "responses",
		"openai_responses": "responses",
		"chat":             "chat.completions",
		"chat_completions": "chat.completions",
		"unknown":          "",
	}
	for input, want := range cases {
		if got := NormalizeOpenAICompatibleAPIFormat(input); got != want {
			t.Fatalf("NormalizeOpenAICompatibleAPIFormat(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTruncateOpenAIUserID_TrimAndCapLength(t *testing.T) {
	longUserID := "  " + strings.Repeat("u", 80) + "  "
	got := TruncateOpenAIUserID(longUserID)
	if len(got) != 64 {
		t.Fatalf("expected truncated length 64, got %d", len(got))
	}
	if got != strings.Repeat("u", 64) {
		t.Fatalf("unexpected truncated user id: %q", got)
	}
}

func TestStreamResponse_ReplaysCanonicalResponse(t *testing.T) {
	resp := &cif.CanonicalResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "Hello"},
			cif.CIFThinkingPart{Type: "thinking", Thinking: "Thinking"},
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_1", ToolName: "Read", ToolArguments: map[string]interface{}{"file": "README.md"}},
		},
		StopReason: cif.StopReasonToolUse,
		Usage:      &cif.CIFUsage{InputTokens: 3, OutputTokens: 4},
	}

	ch := StreamResponse(resp)
	var events []cif.CIFStreamEvent
	for evt := range ch {
		events = append(events, evt)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
	if start, ok := events[0].(cif.CIFStreamStart); !ok || start.ID != "resp_1" {
		t.Fatalf("unexpected start event: %#v", events[0])
	}
	if end, ok := events[len(events)-1].(cif.CIFStreamEnd); !ok || end.StopReason != cif.StopReasonToolUse || end.Usage == nil || end.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected end event: %#v", events[len(events)-1])
	}
}
