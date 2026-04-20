package ingestion

import (
	"testing"

	"omnillm/internal/cif"
)

func TestParseOpenAI_MergesSystemMessagesAndNormalizesToolChoice(t *testing.T) {
	stream := true
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": "You are helpful."},
			map[string]interface{}{"role": "system", "content": "Be concise."},
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "get_weather",
					"parameters": map[string]interface{}{
						"type": "object",
					},
				},
			},
		},
		"tool_choice": map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": "get_weather",
			},
		},
		"stop":   []interface{}{"STOP", "END"},
		"stream": stream,
		"user":   "user123",
	}

	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.SystemPrompt == nil || *req.SystemPrompt != "You are helpful.\n\nBe concise." {
		t.Fatalf("unexpected system prompt: %v", req.SystemPrompt)
	}
	if len(req.Messages) != 1 || req.Messages[0].GetRole() != "user" {
		t.Fatalf("expected only the user message to remain, got %+v", req.Messages)
	}
	if !req.Stream {
		t.Fatal("expected stream=true")
	}
	if len(req.Stop) != 2 || req.Stop[0] != "STOP" || req.Stop[1] != "END" {
		t.Fatalf("unexpected stop sequences: %v", req.Stop)
	}
	if req.UserID == nil || *req.UserID != "user123" {
		t.Fatalf("unexpected user id: %v", req.UserID)
	}

	toolChoice, ok := req.ToolChoice.(map[string]interface{})
	if !ok {
		t.Fatalf("expected function tool choice map, got %T", req.ToolChoice)
	}
	if toolChoice["type"] != "function" || toolChoice["functionName"] != "get_weather" {
		t.Fatalf("unexpected tool choice: %#v", toolChoice)
	}
}

func TestParseOpenAI_PreservesMalformedToolArgumentsAndToolResults(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "assistant",
				"content": "Testing malformed JSON",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call_123",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "test_tool",
							"arguments": "invalid json{",
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_123",
				"content":      "Tool execution failed",
			},
		},
	}

	req, err := ParseOpenAIChatCompletions(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	assistant, ok := req.Messages[0].(cif.CIFAssistantMessage)
	if !ok {
		t.Fatalf("expected assistant message, got %T", req.Messages[0])
	}
	if len(assistant.Content) != 2 {
		t.Fatalf("expected text + tool call content, got %d parts", len(assistant.Content))
	}

	toolCall, ok := assistant.Content[1].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected tool call part, got %T", assistant.Content[1])
	}
	if toolCall.ToolArguments["_unparsable_arguments"] != "invalid json{" {
		t.Fatalf("unexpected malformed tool args payload: %#v", toolCall.ToolArguments)
	}

	toolResultMsg, ok := req.Messages[1].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("expected tool result to become user message, got %T", req.Messages[1])
	}
	resultPart, ok := toolResultMsg.Content[0].(cif.CIFToolResultPart)
	if !ok {
		t.Fatalf("expected tool result part, got %T", toolResultMsg.Content[0])
	}
	if resultPart.ToolCallID != "call_123" || resultPart.Content != "Tool execution failed" {
		t.Fatalf("unexpected tool result part: %#v", resultPart)
	}
}

func TestParseOpenAI_DataURIImageContent(t *testing.T) {
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "What's in this image?"},
					map[string]interface{}{
						"type": "image_url",
						"image_url": map[string]interface{}{
							"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ",
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

	userMsg := req.Messages[0].(cif.CIFUserMessage)
	imagePart, ok := userMsg.Content[1].(cif.CIFImagePart)
	if !ok {
		t.Fatalf("expected image part, got %T", userMsg.Content[1])
	}
	if imagePart.MediaType != "image/png" {
		t.Fatalf("expected image/png, got %q", imagePart.MediaType)
	}
	if imagePart.Data == nil || *imagePart.Data != "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" {
		t.Fatalf("unexpected image data: %v", imagePart.Data)
	}
}

func TestParseAnthropic_NormalizesSystemMetadataAndToolChoice(t *testing.T) {
	payload := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"system": []interface{}{
			map[string]interface{}{"type": "text", "text": "You are helpful."},
			map[string]interface{}{"type": "text", "text": "Be concise."},
		},
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": []interface{}{map[string]interface{}{"type": "text", "text": "Hi"}},
			},
		},
		"metadata": map[string]interface{}{
			"user_id": "user123",
		},
		"stop_sequences": []interface{}{"STOP", "END"},
		"stream":         true,
		"tool_choice":    map[string]interface{}{"type": "auto"},
	}

	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.SystemPrompt == nil || *req.SystemPrompt != "You are helpful.\n\nBe concise." {
		t.Fatalf("unexpected system prompt: %v", req.SystemPrompt)
	}
	if req.UserID == nil || *req.UserID != "user123" {
		t.Fatalf("unexpected user id: %v", req.UserID)
	}
	if !req.Stream {
		t.Fatal("expected stream=true")
	}
	if len(req.Stop) != 2 || req.Stop[0] != "STOP" || req.Stop[1] != "END" {
		t.Fatalf("unexpected stop sequences: %v", req.Stop)
	}
	if req.ToolChoice != "auto" {
		t.Fatalf("unexpected tool choice: %#v", req.ToolChoice)
	}
}

func TestParseAnthropic_PreservesThinkingToolResultsAndNormalizesSchemas(t *testing.T) {
	isError := true
	payload := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{"type": "thinking", "thinking": "Let me think this through."},
					map[string]interface{}{"type": "text", "text": "I'll check that for you."},
					map[string]interface{}{
						"type":  "tool_use",
						"id":    "toolu_01",
						"name":  "get_weather",
						"input": map[string]interface{}{"location": "San Francisco"},
					},
				},
			},
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "toolu_01",
						"content": []interface{}{
							map[string]interface{}{"type": "text", "text": `{"temperature":72}`},
							map[string]interface{}{"type": "text", "text": "Clear skies"},
						},
						"is_error": isError,
					},
				},
			},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"name": "get_weather",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{"type": "string"},
						"units":    map[string]interface{}{"type": "string", "nullable": true},
					},
					"patternProperties": map[string]interface{}{"^x-": map[string]interface{}{"type": "string"}},
					"$schema":           "http://json-schema.org/draft-07/schema#",
				},
			},
		},
		"tool_choice": map[string]interface{}{"type": "tool", "name": "get_weather"},
	}

	req, err := ParseAnthropicMessages(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	assistant := req.Messages[0].(cif.CIFAssistantMessage)
	if len(assistant.Content) != 3 {
		t.Fatalf("expected thinking, text, tool call; got %d parts", len(assistant.Content))
	}
	if _, ok := assistant.Content[0].(cif.CIFThinkingPart); !ok {
		t.Fatalf("expected thinking part, got %T", assistant.Content[0])
	}
	toolCall, ok := assistant.Content[2].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected tool call part, got %T", assistant.Content[2])
	}
	if toolCall.ToolCallID != "toolu_01" || toolCall.ToolName != "get_weather" {
		t.Fatalf("unexpected tool call: %#v", toolCall)
	}

	user := req.Messages[1].(cif.CIFUserMessage)
	toolResult, ok := user.Content[0].(cif.CIFToolResultPart)
	if !ok {
		t.Fatalf("expected tool result part, got %T", user.Content[0])
	}
	if toolResult.Content != "{\"temperature\":72}\n\nClear skies" {
		t.Fatalf("unexpected tool result content: %q", toolResult.Content)
	}
	if toolResult.IsError == nil || !*toolResult.IsError {
		t.Fatalf("expected tool result error flag, got %#v", toolResult.IsError)
	}

	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if _, exists := req.Tools[0].ParametersSchema["patternProperties"]; exists {
		t.Fatal("patternProperties should be removed during normalization")
	}
	if _, exists := req.Tools[0].ParametersSchema["$schema"]; exists {
		t.Fatal("$schema should be removed during normalization")
	}
	properties, ok := req.Tools[0].ParametersSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties map, got %#v", req.Tools[0].ParametersSchema["properties"])
	}
	units, ok := properties["units"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected units schema map, got %#v", properties["units"])
	}
	typeList, ok := units["type"].([]interface{})
	if !ok || len(typeList) != 2 || typeList[0] != "string" || typeList[1] != "null" {
		t.Fatalf("expected nullable type normalization, got %#v", units["type"])
	}

	toolChoice, ok := req.ToolChoice.(map[string]interface{})
	if !ok || toolChoice["functionName"] != "get_weather" {
		t.Fatalf("unexpected tool choice: %#v", req.ToolChoice)
	}
}
