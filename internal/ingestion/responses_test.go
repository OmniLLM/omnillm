package ingestion

import (
	"testing"

	"omnillm/internal/cif"
)

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

	req, err := ParseResponsesPayload(payload)
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
	req, err := ParseResponsesPayload(map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": "Hello from responses",
	})
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

func TestParseResponsesPayload_FunctionCallRequiresIdentifier(t *testing.T) {
	_, err := ParseResponsesPayload(map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type":      "function_call",
				"name":      "get_weather",
				"arguments": `{"location":"Boston"}`,
			},
		},
	})
	if err == nil {
		t.Fatal("expected missing function_call id to fail")
	}
}
