package ingestion

import (
	"encoding/json"
	"reflect"
	"testing"

	"omnillm/internal/cif"
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

	if cif.PlainSystemText(req.System) != "Be terse." {
		t.Fatalf("unexpected system prompt: %v", req.System)
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
	if cif.PlainSystemText(systemMsg.Content) != "You are a coding assistant." {
		t.Fatalf("unexpected system content: %#v", systemMsg.Content)
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
					map[string]interface{}{"type": "totally_unknown_block_xyz", "text": "ignored"},
				},
			},
		},
	}))
	if err == nil {
		t.Fatal("expected unknown content block type to fail")
	}
}

func TestParseResponsesPayload_AcceptsInputImage(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type": "message",
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "input_image", "image_url": "https://example.com/img.png"},
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("expected input_image to be accepted, got error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
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

func TestParseResponsesPayload_MergesAssistantFunctionCallsIntoSingleTurn(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type":    "message",
				"role":    "assistant",
				"content": []interface{}{map[string]interface{}{"type": "output_text", "text": "Let me check."}},
			},
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "tool_a",
				"arguments": `{"x":1}`,
			},
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_2",
				"name":      "tool_b",
				"arguments": `{"y":2}`,
			},
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_1",
				"name":    "tool_a",
				"output":  "one",
			},
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_2",
				"name":    "tool_b",
				"output":  "two",
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("expected assistant turn plus two tool results, got %d messages", len(req.Messages))
	}

	assistantMsg, ok := req.Messages[0].(cif.CIFAssistantMessage)
	if !ok {
		t.Fatalf("expected first message to be assistant, got %T", req.Messages[0])
	}
	if len(assistantMsg.Content) != 3 {
		t.Fatalf("expected assistant turn to include text plus two tool calls, got %d parts", len(assistantMsg.Content))
	}

	textPart, ok := assistantMsg.Content[0].(cif.CIFTextPart)
	if !ok || textPart.Text != "Let me check." {
		t.Fatalf("unexpected assistant text part: %#v", assistantMsg.Content[0])
	}

	firstToolCall, ok := assistantMsg.Content[1].(cif.CIFToolCallPart)
	if !ok || firstToolCall.ToolCallID != "call_1" || firstToolCall.ToolName != "tool_a" {
		t.Fatalf("unexpected first tool call: %#v", assistantMsg.Content[1])
	}

	secondToolCall, ok := assistantMsg.Content[2].(cif.CIFToolCallPart)
	if !ok || secondToolCall.ToolCallID != "call_2" || secondToolCall.ToolName != "tool_b" {
		t.Fatalf("unexpected second tool call: %#v", assistantMsg.Content[2])
	}

	toolResultOne := req.Messages[1].(cif.CIFUserMessage).Content[0].(cif.CIFToolResultPart)
	if toolResultOne.ToolCallID != "call_1" || toolResultOne.Content != "one" {
		t.Fatalf("unexpected first tool result: %#v", toolResultOne)
	}

	toolResultTwo := req.Messages[2].(cif.CIFUserMessage).Content[0].(cif.CIFToolResultPart)
	if toolResultTwo.ToolCallID != "call_2" || toolResultTwo.Content != "two" {
		t.Fatalf("unexpected second tool result: %#v", toolResultTwo)
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

func TestParseResponsesPayload_IgnoresReasoningItems(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type": "reasoning",
				"id":   "rs_123",
			},
			map[string]interface{}{
				"type":    "message",
				"role":    "assistant",
				"content": []interface{}{map[string]interface{}{"type": "output_text", "text": "Plan updated"}},
			},
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "Glob",
				"arguments": `{"pattern":"src/**"}`,
			},
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_1",
				"name":    "Glob",
				"output":  "src/a.ts",
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("expected assistant turn plus tool result, got %d messages", len(req.Messages))
	}
}

func TestParseResponsesPayload_IgnoresAdditionalToolsItems(t *testing.T) {
	// Codex >=0.144 prepends an "additional_tools" input item; it must be
	// skipped as a message (like "reasoning") AND its tool defs extracted into
	// the CIF tool set. Regression for the "unknown input item type:
	// additional_tools" 400 and the "tools:0" empty-toolset bug on translated
	// backends (e.g. gpt-5.6-sol).
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type": "additional_tools",
				"role": "developer",
				"id":   "at_123",
				"tools": []interface{}{
					map[string]interface{}{
						"name":        "shell",
						"description": "Run a shell command",
						"parameters":  map[string]interface{}{"type": "object"},
					},
					map[string]interface{}{
						"type": "namespace",
						"name": "fs",
						"tools": []interface{}{
							map[string]interface{}{
								"name":        "read",
								"description": "Read a file",
								"parameters":  map[string]interface{}{"type": "object"},
							},
						},
					},
					map[string]interface{}{
						"type": "namespace",
						"name": "duplicate",
						"tools": []interface{}{
							map[string]interface{}{
								"name":        "read",
								"description": "Duplicate should be ignored",
								"parameters":  map[string]interface{}{"type": "object"},
							},
						},
					},
				},
			},
			map[string]interface{}{
				"type":    "message",
				"role":    "user",
				"content": []interface{}{map[string]interface{}{"type": "input_text", "text": "hi"}},
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected only the user message, got %d messages", len(req.Messages))
	}
	if len(req.Tools) != 2 {
		t.Fatalf("expected 2 extracted tools, got %d: %#v", len(req.Tools), req.Tools)
	}
	names := map[string]bool{}
	for _, tl := range req.Tools {
		names[tl.Name] = true
	}
	if !names["shell"] || !names["read"] {
		t.Fatalf("expected tools shell + read, got %#v", names)
	}
	if names["fs__read"] {
		t.Fatalf("namespace prefix changed callable name: %#v", names)
	}
}

func TestParseResponsesPayload_PreservesCodexExecToolName(t *testing.T) {
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.6-sol",
		"input": []interface{}{
			map[string]interface{}{
				"type": "additional_tools",
				"tools": []interface{}{
					map[string]interface{}{
						"type": "namespace",
						"name": "functions",
						"tools": []interface{}{
							map[string]interface{}{
								"type":        "custom",
								"name":        "exec",
								"description": "Run a shell command",
								"parameters":  map[string]interface{}{"type": "object"},
							},
						},
					},
				},
			},
			map[string]interface{}{"type": "message", "role": "user", "content": "run it"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "exec" {
		t.Fatalf("Codex tool name changed: %#v", req.Tools)
	}
	if req.Tools[0].ToolKind != cif.CIFToolKindCustom {
		t.Fatalf("Codex custom tool kind lost: %#v", req.Tools[0])
	}
}

func TestParseResponsesPayload_NormalizesCustomToolHistory(t *testing.T) {
	rawInput := "*** Update File: settings.json\n@@\n- old\n+ new"
	contentOutput := []interface{}{
		map[string]interface{}{"type": "input_text", "text": "failed"},
		map[string]interface{}{"type": "input_image", "image_url": "https://example.com/error.png"},
		map[string]interface{}{"type": "input_image", "file_id": "file_image_123"},
		map[string]interface{}{"type": "input_file", "file_id": "file_123"},
	}
	req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
		"model": "gpt-5.4-mini",
		"input": []interface{}{
			map[string]interface{}{
				"type":    "message",
				"role":    "assistant",
				"content": "Applying the change.",
			},
			map[string]interface{}{
				"type":    "custom_tool_call",
				"call_id": "call_patch",
				"name":    "ApplyPatch",
				"input":   rawInput,
			},
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_read",
				"name":      "Read",
				"arguments": `{"file_path":"settings.json"}`,
			},
			map[string]interface{}{
				"type":    "custom_tool_call_output",
				"call_id": "call_patch",
				"output":  "patch failed",
			},
			map[string]interface{}{
				"type":    "custom_tool_call",
				"call_id": "call_empty",
				"name":    "ApplyPatch",
				"input":   "",
			},
			map[string]interface{}{
				"type":    "custom_tool_call_output",
				"call_id": "call_empty",
				"output":  contentOutput,
			},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"type":        "custom",
				"name":        "ApplyPatch",
				"description": "Apply a patch",
				"format":      map[string]interface{}{"type": "text"},
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assistant := req.Messages[0].(cif.CIFAssistantMessage)
	if len(assistant.Content) != 3 {
		t.Fatalf("expected text plus two calls, got %#v", assistant.Content)
	}
	customCall := assistant.Content[1].(cif.CIFToolCallPart)
	if customCall.ToolCallID != "call_patch" || customCall.ToolName != "ApplyPatch" {
		t.Fatalf("unexpected custom call: %#v", customCall)
	}
	if customCall.ToolKind != cif.CIFToolKindCustom || customCall.RawInput == nil || *customCall.RawInput != rawInput {
		t.Fatalf("native custom call data changed: %#v", customCall)
	}
	if !reflect.DeepEqual(customCall.ToolArguments, map[string]interface{}{"input": rawInput}) {
		t.Fatalf("custom input changed: %#v", customCall.ToolArguments)
	}
	functionCall := assistant.Content[2].(cif.CIFToolCallPart)
	if !reflect.DeepEqual(functionCall.ToolArguments, map[string]interface{}{"file_path": "settings.json"}) {
		t.Fatalf("function arguments changed: %#v", functionCall.ToolArguments)
	}

	firstResult := req.Messages[1].(cif.CIFUserMessage).Content[0].(cif.CIFToolResultPart)
	if firstResult.ToolCallID != "call_patch" || firstResult.Content != "patch failed" {
		t.Fatalf("unexpected string result: %#v", firstResult)
	}
	if firstResult.ToolKind != cif.CIFToolKindCustom || firstResult.CustomOutput != "patch failed" {
		t.Fatalf("native custom output changed: %#v", firstResult)
	}
	secondAssistant := req.Messages[2].(cif.CIFAssistantMessage)
	emptyCall := secondAssistant.Content[0].(cif.CIFToolCallPart)
	if emptyCall.ToolArguments["input"] != "" {
		t.Fatalf("empty custom input was not preserved: %#v", emptyCall.ToolArguments)
	}
	listResult := req.Messages[3].(cif.CIFUserMessage).Content[0].(cif.CIFToolResultPart)
	wantOutput, _ := json.Marshal(contentOutput)
	if listResult.ToolCallID != "call_empty" || listResult.Content != string(wantOutput) {
		t.Fatalf("unexpected content-list result: %#v", listResult)
	}

	if len(req.Tools) != 1 {
		t.Fatalf("expected one custom tool, got %#v", req.Tools)
	}
	wantSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{"type": "string"},
		},
		"required":             []interface{}{"input"},
		"additionalProperties": false,
	}
	if !reflect.DeepEqual(req.Tools[0].ParametersSchema, wantSchema) {
		t.Fatalf("unexpected custom tool schema: %#v", req.Tools[0].ParametersSchema)
	}
}

func TestParseResponsesPayload_ValidatesCustomToolItems(t *testing.T) {
	tests := []struct {
		name string
		item map[string]interface{}
	}{
		{"call-missing-id", map[string]interface{}{"type": "custom_tool_call", "name": "ApplyPatch", "input": ""}},
		{"call-missing-name", map[string]interface{}{"type": "custom_tool_call", "call_id": "call_1", "input": ""}},
		{"call-missing-input", map[string]interface{}{"type": "custom_tool_call", "call_id": "call_1", "name": "ApplyPatch"}},
		{"call-non-string-input", map[string]interface{}{"type": "custom_tool_call", "call_id": "call_1", "name": "ApplyPatch", "input": map[string]interface{}{}}},
		{"output-missing-id", map[string]interface{}{"type": "custom_tool_call_output", "output": ""}},
		{"output-missing-value", map[string]interface{}{"type": "custom_tool_call_output", "call_id": "call_1"}},
		{"output-invalid-scalar", map[string]interface{}{"type": "custom_tool_call_output", "call_id": "call_1", "output": 42}},
		{"output-invalid-member", map[string]interface{}{"type": "custom_tool_call_output", "call_id": "call_1", "output": []interface{}{map[string]interface{}{"type": "unknown"}}}},
		{"output-text-missing-text", map[string]interface{}{"type": "custom_tool_call_output", "call_id": "call_1", "output": []interface{}{map[string]interface{}{"type": "input_text"}}}},
		{"output-image-missing-url", map[string]interface{}{"type": "custom_tool_call_output", "call_id": "call_1", "output": []interface{}{map[string]interface{}{"type": "input_image"}}}},
		{"output-file-missing-reference", map[string]interface{}{"type": "custom_tool_call_output", "call_id": "call_1", "output": []interface{}{map[string]interface{}{"type": "input_file"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
				"model": "gpt-5.4-mini",
				"input": []interface{}{test.item},
			}))
			if err == nil {
				t.Fatal("expected invalid custom item to fail")
			}
		})
	}
}

func TestParseResponsesPayload_AcceptsEmptyCustomOutputValues(t *testing.T) {
	for _, output := range []interface{}{"", []interface{}{}} {
		req, err := ParseResponsesPayload(mustRawR(t, map[string]interface{}{
			"model": "gpt-5.4-mini",
			"input": []interface{}{
				map[string]interface{}{
					"type":    "custom_tool_call_output",
					"call_id": "call_1",
					"output":  output,
				},
			},
		}))
		if err != nil {
			t.Fatalf("output %#v: %v", output, err)
		}
		result := req.Messages[0].(cif.CIFUserMessage).Content[0].(cif.CIFToolResultPart)
		if outputList, ok := output.([]interface{}); ok {
			encoded, _ := json.Marshal(outputList)
			if result.Content != string(encoded) {
				t.Fatalf("empty list encoded as %q", result.Content)
			}
			continue
		}
		if result.Content != "" {
			t.Fatalf("empty string changed to %q", result.Content)
		}
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
