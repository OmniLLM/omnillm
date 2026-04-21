package ingestion

import (
	"encoding/json"
	"omnillm/internal/cif"
	"testing"
)

func mustRawR(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, _ := json.Marshal(v)
	return b
}

func TestParseResponsesPayload_TranslatesInstructionsMessagesAndTools(t *testing.T) {
	stream := true
	maxOutputTokens := 256
	payload := map[string]interface{}{
		"model":             "gpt-5.4-mini",
		"instructions":      "Be terse.",
		"stream":            stream,
		"max_output_tokens": maxOutputTokens,
		"input": []interface{}{
			map[string]interface{}{
				"type":    "message",
				"role":    "user",
				"content": []interface{}{map[string]interface{}{"type": "output_text", "text": "Hello"}},
			},
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_123",
				"name":      "get_weather",
				"arguments": `{"location":"Boston"}`,
			},
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_123",
				"name":    "get_weather",
				"output":  "Sunny",
			},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"type":        "function",
				"name":        "get_weather",
				"description": "Get weather",
				"parameters": map[string]interface{}{
					"type": "object",
				},
			},
			map[string]interface{}{
				"type": "function",
			},
		},
		"tool_choice": map[string]interface{}{
			"function": map[string]interface{}{
				"name": "get_weather",
			},
		},
	}

	req, err := ParseResponsesPayload(mustRawR(t, payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.SystemPrompt == nil || *req.SystemPrompt != "Be terse." {
		t.Fatalf("unexpected system prompt: %v", req.SystemPrompt)
	}
	if !req.Stream {
		t.Fatal("expected stream=true")
	}
	if req.MaxTokens == nil || *req.MaxTokens != maxOutputTokens {
		t.Fatalf("unexpected max tokens: %v", req.MaxTokens)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(req.Messages))
	}

	userMsg, ok := req.Messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected first message to be user, got %T", req.Messages[0])
	}
	textPart, ok := userMsg.Content[0].(cif.CIFTextPart)
	if !ok || textPart.Text != "Hello" {
		t.Fatalf("unexpected user message content: %#v", userMsg.Content[0])
	}

	assistantMsg, ok := req.Messages[1].(cif.CIFAssistantMessage)
	if !ok {
		t.Fatalf("expected function_call to become assistant message, got %T", req.Messages[1])
	}
	toolCall, ok := assistantMsg.Content[0].(cif.CIFToolCallPart)
	if !ok || toolCall.ToolCallID != "call_123" || toolCall.ToolName != "get_weather" {
		t.Fatalf("unexpected tool call: %#v", assistantMsg.Content[0])
	}

	toolResultMsg, ok := req.Messages[2].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected function_call_output to become user message, got %T", req.Messages[2])
	}
	toolResult, ok := toolResultMsg.Content[0].(cif.CIFToolResultPart)
	if !ok || toolResult.ToolCallID != "call_123" || toolResult.Content != "Sunny" {
		t.Fatalf("unexpected tool result: %#v", toolResultMsg.Content[0])
	}

	if len(req.Tools) != 1 || req.Tools[0].Name != "get_weather" {
		t.Fatalf("expected only the valid tool to remain, got %#v", req.Tools)
	}
	toolChoice, ok := req.ToolChoice.(map[string]interface{})
	if !ok || toolChoice["functionName"] != "get_weather" {
		t.Fatalf("unexpected tool choice: %#v", req.ToolChoice)
	}
}

func TestParseResponsesPayload_AcceptsStringInput(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": "Hello from responses",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	userMsg, ok := req.Messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected user message, got %T", req.Messages[0])
	}
	textPart := userMsg.Content[0].(cif.CIFTextPart)
	if textPart.Text != "Hello from responses" {
		t.Fatalf("unexpected text content: %q", textPart.Text)
	}
}

func TestParseResponsesPayload_AcceptsDeveloperRole(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type":    "message",
				"role":    "developer",
				"content": []interface{}{map[string]interface{}{"type": "input_text", "text": "You are a coding assistant."}},
			},
			map[string]interface{}{
				"type":    "message",
				"role":    "user",
				"content": "Hello",
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	systemMsg, ok := req.Messages[0].(cif.CIFSystemMessage)
	if !ok {
		t.Fatalf("expected first message to be system, got %T", req.Messages[0])
	}
	if systemMsg.Content != "You are a coding assistant." {
		t.Fatalf("unexpected system content: %q", systemMsg.Content)
	}
	userMsg, ok := req.Messages[1].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected second message to be user, got %T", req.Messages[1])
	}
	textPart := userMsg.Content[0].(cif.CIFTextPart)
	if textPart.Text != "Hello" {
		t.Fatalf("unexpected user text content: %q", textPart.Text)
	}
}

func TestParseResponsesPayload_RejectsUnknownContentBlockType(t *testing.T) {
	_, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type": "message",
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "input_image", "text": "ignored"},
				},
			},
		},
	}))
	if err == nil {
		t.Fatal("expected unknown content block type to fail")
	}
}

func TestParseResponsesPayload_FunctionCallOutputBecomesToolResult(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_123",
				"name":    "get_weather",
				"output":  "Sunny",
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	userMsg, ok := req.Messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected user message, got %T", req.Messages[0])
	}
	toolResult, ok := userMsg.Content[0].(cif.CIFToolResultPart)
	if !ok {
		t.Fatalf("expected tool result part, got %#v", userMsg.Content[0])
	}
	if toolResult.ToolCallID != "call_123" || toolResult.ToolName != "get_weather" || toolResult.Content != "Sunny" {
		t.Fatalf("unexpected tool result: %#v", toolResult)
	}
}

func TestParseResponsesPayload_FunctionCallRequiresIdentifier(t *testing.T) {
	_, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type":      "function_call",
				"name":      "get_weather",
				"arguments": `{"location":"Boston"}`,
			},
		},
	}))
	if err == nil {
		t.Fatal("expected missing function_call id to fail")
	}
}

func TestParseResponsesPayload_InfersMissingMessageType(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": []interface{}{map[string]interface{}{"type": "text", "text": "Hello from Droid"}},
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	userMsg, ok := req.Messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected user message, got %T", req.Messages[0])
	}
	textPart := userMsg.Content[0].(cif.CIFTextPart)
	if textPart.Text != "Hello from Droid" {
		t.Fatalf("unexpected text content: %q", textPart.Text)
	}
}

func TestParseResponsesPayload_InfersMissingFunctionCallType(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"id":        "call_123",
				"name":      "get_weather",
				"arguments": `{"location":"Boston"}`,
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assistantMsg := req.Messages[0].(cif.CIFAssistantMessage)
	toolCall := assistantMsg.Content[0].(cif.CIFToolCallPart)
	if toolCall.ToolCallID != "call_123" || toolCall.ToolName != "get_weather" {
		t.Fatalf("unexpected tool call: %#v", toolCall)
	}
}

func TestParseResponsesPayload_InfersMissingFunctionCallOutputType(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"call_id": "call_123",
				"name":    "get_weather",
				"output":  "Sunny",
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userMsg := req.Messages[0].(cif.CIFUserMessage)
	toolResult := userMsg.Content[0].(cif.CIFToolResultPart)
	if toolResult.ToolCallID != "call_123" || toolResult.Content != "Sunny" {
		t.Fatalf("unexpected tool result: %#v", toolResult)
	}
}

func TestParseResponsesPayload_RejectsMalformedInputItem(t *testing.T) {
	_, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{"not-a-map"},
	}))
	if err == nil {
		t.Fatal("expected malformed input item to fail")
	}
}
