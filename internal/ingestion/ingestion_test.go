package ingestion

import (
	"testing"

	"omnimodel/internal/cif"
)

// ─── ParseOpenAIChatCompletions ───

func TestParseOpenAI_SimpleUserMessage(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %q", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].GetRole() != "user" {
		t.Errorf("expected user role, got %q", req.Messages[0].GetRole())
	}
}

func TestParseOpenAI_SystemMessage(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": "You are helpful."},
			map[string]interface{}{"role": "user", "content": "Hi"},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SystemPrompt == nil || *req.SystemPrompt != "You are helpful." {
		t.Fatalf("expected system prompt to be preserved, got %v", req.SystemPrompt)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 non-system message, got %d", len(req.Messages))
	}
	if req.Messages[0].GetRole() != "user" {
		t.Errorf("expected first remaining message to be user, got %q", req.Messages[0].GetRole())
	}
}

func TestParseOpenAI_AssistantMessage(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "What is 2+2?"},
			map[string]interface{}{"role": "assistant", "content": "It is 4."},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Messages[1].GetRole() != "assistant" {
		t.Errorf("expected assistant, got %q", req.Messages[1].GetRole())
	}
}

func TestParseOpenAI_ToolCallInAssistantMessage(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "assistant",
				"content": "",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call_abc",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "get_weather",
							"arguments": `{"location":"NYC"}`,
						},
					},
				},
			},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assistantMsg, ok := req.Messages[0].(cif.CIFAssistantMessage)
	if !ok {
		t.Fatalf("expected CIFAssistantMessage, got %T", req.Messages[0])
	}
	if len(assistantMsg.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(assistantMsg.Content))
	}
	toolCall, ok := assistantMsg.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected CIFToolCallPart, got %T", assistantMsg.Content[0])
	}
	if toolCall.ToolName != "get_weather" {
		t.Errorf("expected get_weather, got %q", toolCall.ToolName)
	}
	if toolCall.ToolCallID != "call_abc" {
		t.Errorf("expected call_abc, got %q", toolCall.ToolCallID)
	}
}

func TestParseOpenAI_ToolCallInAssistantMessage_WithCallIDAlias(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []interface{}{
					map[string]interface{}{
						"call_id": "call_alias",
						"type":    "function",
						"function": map[string]interface{}{
							"name":      "get_weather",
							"arguments": `{"location":"SF"}`,
						},
					},
				},
			},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assistantMsg, ok := req.Messages[0].(cif.CIFAssistantMessage)
	if !ok {
		t.Fatalf("expected CIFAssistantMessage, got %T", req.Messages[0])
	}
	if len(assistantMsg.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(assistantMsg.Content))
	}
	toolCall, ok := assistantMsg.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected CIFToolCallPart, got %T", assistantMsg.Content[0])
	}
	if toolCall.ToolCallID != "call_alias" {
		t.Errorf("expected call_alias, got %q", toolCall.ToolCallID)
	}
}

func TestParseOpenAI_Tools(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Get weather"},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "get_weather",
					"description": "Get weather for a city",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %q", req.Tools[0].Name)
	}
}

func TestParseOpenAI_ImageURLContent(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "What's in this image?",
					},
					map[string]interface{}{
						"type": "image_url",
						"image_url": map[string]interface{}{
							"url": "https://example.com/img.jpg",
						},
					},
				},
			},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userMsg, ok := req.Messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected CIFUserMessage, got %T", req.Messages[0])
	}
	if len(userMsg.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(userMsg.Content))
	}
	_, isText := userMsg.Content[0].(cif.CIFTextPart)
	if !isText {
		t.Errorf("expected CIFTextPart at index 0, got %T", userMsg.Content[0])
	}
	imgPart, isImage := userMsg.Content[1].(cif.CIFImagePart)
	if !isImage {
		t.Errorf("expected CIFImagePart at index 1, got %T", userMsg.Content[1])
	}
	if imgPart.URL == nil || *imgPart.URL != "https://example.com/img.jpg" {
		t.Errorf("unexpected image URL: %v", imgPart.URL)
	}
}

func TestParseOpenAI_StreamFlag(t *testing.T) {
	stream := true
	_ = stream
	payload := map[string]interface{}{
		"model":  "gpt-4o",
		"stream": true,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !req.Stream {
		t.Error("expected Stream=true")
	}
}

func TestParseOpenAI_StopString(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"stop":  "END",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hi"},
		},
	}
	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Stop) != 1 || req.Stop[0] != "END" {
		t.Errorf("expected stop=[END], got %v", req.Stop)
	}
}

func TestParseOpenAI_UnknownRole(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{"role": "function", "content": "result"},
		},
	}
	_, err := ParseOpenAIChatCompletions(payload)
	if err == nil {
		t.Error("expected error for unknown role 'function'")
	}
}

// ─── ParseAnthropicMessages ───

func TestParseAnthropic_SimpleUserMessage(t *testing.T) {
	payload := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "Hello"},
				},
			},
		},
	}
	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("unexpected model: %q", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].GetRole() != "user" {
		t.Errorf("expected user, got %q", req.Messages[0].GetRole())
	}
}

func TestParseAnthropic_SystemPrompt(t *testing.T) {
	payload := map[string]interface{}{
		"model":  "claude-3-5-sonnet-20241022",
		"system": "You are a pirate.",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "Hello"},
				},
			},
		},
	}
	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SystemPrompt == nil || *req.SystemPrompt != "You are a pirate." {
		t.Errorf("unexpected system prompt: %v", req.SystemPrompt)
	}
}

func TestParseAnthropic_ToolUseBlock(t *testing.T) {
	payload := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_use",
						"tool_use_id": "toolu_01",
						"name":        "get_weather",
						"input":       map[string]interface{}{"location": "NYC"},
					},
				},
			},
		},
	}
	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assistantMsg, ok := req.Messages[0].(cif.CIFAssistantMessage)
	if !ok {
		t.Fatalf("expected CIFAssistantMessage, got %T", req.Messages[0])
	}
	toolPart, ok := assistantMsg.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected CIFToolCallPart, got %T", assistantMsg.Content[0])
	}
	if toolPart.ToolName != "get_weather" {
		t.Errorf("expected get_weather, got %q", toolPart.ToolName)
	}
}

func TestParseAnthropic_ToolResultBlock(t *testing.T) {
	payload := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "toolu_01",
						"content":     "Sunny, 72°F",
					},
				},
			},
		},
	}
	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userMsg, ok := req.Messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected CIFUserMessage, got %T", req.Messages[0])
	}
	resultPart, ok := userMsg.Content[0].(cif.CIFToolResultPart)
	if !ok {
		t.Fatalf("expected CIFToolResultPart, got %T", userMsg.Content[0])
	}
	if resultPart.ToolCallID != "toolu_01" {
		t.Errorf("expected toolu_01, got %q", resultPart.ToolCallID)
	}
	if resultPart.Content != "Sunny, 72°F" {
		t.Errorf("unexpected content: %q", resultPart.Content)
	}
}

func TestParseAnthropic_ToolResultBlockFallsBackToID(t *testing.T) {
	payload := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":    "tool_result",
						"id":      "toolu_fallback",
						"name":    "Read",
						"content": "fallback id result",
					},
				},
			},
		},
	}
	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userMsg, ok := req.Messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected CIFUserMessage, got %T", req.Messages[0])
	}
	resultPart, ok := userMsg.Content[0].(cif.CIFToolResultPart)
	if !ok {
		t.Fatalf("expected CIFToolResultPart, got %T", userMsg.Content[0])
	}
	if resultPart.ToolCallID != "toolu_fallback" {
		t.Errorf("expected fallback tool call id toolu_fallback, got %q", resultPart.ToolCallID)
	}
	if resultPart.ToolName != "Read" {
		t.Errorf("expected tool name Read, got %q", resultPart.ToolName)
	}
}

func TestParseAnthropic_ImageBlock(t *testing.T) {
	payload := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type":       "base64",
							"media_type": "image/png",
							"data":       "iVBORw0KGgo=",
						},
					},
				},
			},
		},
	}
	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userMsg, ok := req.Messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected CIFUserMessage, got %T", req.Messages[0])
	}
	imgPart, ok := userMsg.Content[0].(cif.CIFImagePart)
	if !ok {
		t.Fatalf("expected CIFImagePart, got %T", userMsg.Content[0])
	}
	if imgPart.MediaType != "image/png" {
		t.Errorf("expected image/png, got %q", imgPart.MediaType)
	}
	if imgPart.Data == nil || *imgPart.Data != "iVBORw0KGgo=" {
		t.Errorf("unexpected image data: %v", imgPart.Data)
	}
}

func TestParseAnthropic_Tools(t *testing.T) {
	payload := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "Use tool"},
				},
			},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "get_weather",
				"description": "Gets weather",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}
	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Name != "get_weather" {
		t.Errorf("expected get_weather, got %q", req.Tools[0].Name)
	}
}

func TestParseAnthropic_MaxTokens(t *testing.T) {
	maxTok := 1024
	_ = maxTok
	payload := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": float64(1024),
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": []interface{}{map[string]interface{}{"type": "text", "text": "Hi"}},
			},
		},
	}
	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 1024 {
		t.Errorf("expected MaxTokens=1024, got %v", req.MaxTokens)
	}
}
