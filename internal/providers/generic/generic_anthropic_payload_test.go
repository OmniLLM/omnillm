package generic

import (
	"testing"

	"omnimodel/internal/ingestion"
)

func TestQwen36PlusAnthropicAgentToolRequestShape(t *testing.T) {
	provider := NewGenericProvider("alibaba", "qwen36-anthropic-test", "Qwen 3.6 Plus")
	provider.baseURL = "https://dashscope.aliyuncs.com/apps/anthropic/v1"
	adapter := &GenericAdapter{provider: provider}

	description := "Explore codebase structure"
	requestPayload := map[string]interface{}{
		"model": "qwen3.6-plus",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Inspect the repository and report the architecture.",
					},
				},
			},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "Agent",
				"description": description,
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"description": map[string]interface{}{"type": "string"},
						"prompt":      map[string]interface{}{"type": "string"},
						"subagent_type": map[string]interface{}{
							"type": "string",
						},
						"model": map[string]interface{}{
							"type": "string",
							"enum": []interface{}{"haiku", "sonnet", "opus"},
						},
						"run_in_background": map[string]interface{}{"type": "boolean"},
					},
					"required": []interface{}{"description", "prompt"},
				},
			},
		},
		"tool_choice": map[string]interface{}{"type": "tool", "name": "Agent"},
		"max_tokens":  4096,
		"stream":      true,
	}

	req, err := ingestion.ParseAnthropicMessages(requestPayload)
	if err != nil {
		t.Fatalf("ParseAnthropicMessages() error = %v", err)
	}

	built := adapter.buildAnthropicPayload(req)

	if got := built["model"]; got != "qwen3.6-plus" {
		t.Fatalf("model = %v, want qwen3.6-plus", got)
	}

	if got := built["max_tokens"]; got != 4096 {
		t.Fatalf("max_tokens = %v, want 4096", got)
	}

	messages, ok := built["messages"].([]map[string]interface{})
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v, want single anthropic user message", built["messages"])
	}

	if got := messages[0]["role"]; got != "user" {
		t.Fatalf("message role = %v, want user", got)
	}

	content, ok := messages[0]["content"].([]map[string]interface{})
	if !ok || len(content) != 1 {
		t.Fatalf("message content = %#v, want single text block", messages[0]["content"])
	}

	if got := content[0]["text"]; got != "Inspect the repository and report the architecture." {
		t.Fatalf("message text = %v", got)
	}

	tools, ok := built["tools"].([]map[string]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want single Agent tool", built["tools"])
	}

	tool := tools[0]
	if got := tool["name"]; got != "Agent" {
		t.Fatalf("tool name = %v, want Agent", got)
	}
	if got := tool["description"]; got != description {
		t.Fatalf("tool description = %v, want %q", got, description)
	}

	schema, ok := tool["input_schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("input_schema = %#v, want object", tool["input_schema"])
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties = %#v, want object", schema["properties"])
	}

	for _, key := range []string{"description", "prompt", "subagent_type", "model", "run_in_background"} {
		if _, exists := properties[key]; !exists {
			t.Fatalf("expected input_schema.properties[%q] to exist, got %#v", key, properties)
		}
	}

	modelSchema, ok := properties["model"].(map[string]interface{})
	if !ok {
		t.Fatalf("model schema = %#v, want object", properties["model"])
	}
	enumValues, ok := modelSchema["enum"].([]interface{})
	if !ok || len(enumValues) != 3 || enumValues[0] != "haiku" || enumValues[1] != "sonnet" || enumValues[2] != "opus" {
		t.Fatalf("model enum = %#v, want [haiku sonnet opus]", modelSchema["enum"])
	}

	required, ok := schema["required"].([]interface{})
	if !ok || len(required) != 2 || required[0] != "description" || required[1] != "prompt" {
		t.Fatalf("required = %#v, want [description prompt]", schema["required"])
	}

	toolChoice, ok := built["tool_choice"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool_choice = %#v, want object", built["tool_choice"])
	}
	if got := toolChoice["type"]; got != "tool" {
		t.Fatalf("tool_choice.type = %v, want tool", got)
	}
	if got := toolChoice["name"]; got != "Agent" {
		t.Fatalf("tool_choice.name = %v, want Agent", got)
	}
	if _, exists := built["stream"]; exists {
		t.Fatalf("buildAnthropicPayload should not set stream directly, got %#v", built["stream"])
	}
}

func TestQwen36PlusAnthropicAgentToolLoopShape(t *testing.T) {
	provider := NewGenericProvider("alibaba", "qwen36-anthropic-test", "Qwen 3.6 Plus")
	provider.baseURL = "https://dashscope.aliyuncs.com/apps/anthropic/v1"
	adapter := &GenericAdapter{provider: provider}

	requestPayload := map[string]interface{}{
		"model": "qwen3.6-plus",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "I'll delegate that."},
					map[string]interface{}{
						"type":  "tool_use",
						"id":    "call_agent_123",
						"name":  "Agent",
						"input": map[string]interface{}{"description": "Explore codebase structure", "prompt": "Inspect src and summarize modules."},
					},
				},
			},
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "call_agent_123",
						"name":        "Agent",
						"content":     "Exploration complete.",
					},
				},
			},
		},
	}

	req, err := ingestion.ParseAnthropicMessages(requestPayload)
	if err != nil {
		t.Fatalf("ParseAnthropicMessages() error = %v", err)
	}

	built := adapter.buildAnthropicPayload(req)

	messages, ok := built["messages"].([]map[string]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want assistant and user entries", built["messages"])
	}

	assistantContent, ok := messages[0]["content"].([]map[string]interface{})
	if !ok || len(assistantContent) != 2 {
		t.Fatalf("assistant content = %#v, want text and tool_use", messages[0]["content"])
	}

	toolUse := assistantContent[1]
	if got := toolUse["type"]; got != "tool_use" {
		t.Fatalf("assistant tool block type = %v, want tool_use", got)
	}
	if got := toolUse["id"]; got != "call_agent_123" {
		t.Fatalf("assistant tool block id = %v, want call_agent_123", got)
	}
	if got := toolUse["name"]; got != "Agent" {
		t.Fatalf("assistant tool block name = %v, want Agent", got)
	}

	toolInput, ok := toolUse["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("assistant tool block input = %#v, want object", toolUse["input"])
	}
	if got := toolInput["description"]; got != "Explore codebase structure" {
		t.Fatalf("assistant tool input description = %v", got)
	}
	if got := toolInput["prompt"]; got != "Inspect src and summarize modules." {
		t.Fatalf("assistant tool input prompt = %v", got)
	}

	userContent, ok := messages[1]["content"].([]map[string]interface{})
	if !ok || len(userContent) != 1 {
		t.Fatalf("user content = %#v, want single tool_result block", messages[1]["content"])
	}

	toolResult := userContent[0]
	if got := toolResult["type"]; got != "tool_result" {
		t.Fatalf("user tool result type = %v, want tool_result", got)
	}
	if got := toolResult["tool_use_id"]; got != "call_agent_123" {
		t.Fatalf("user tool result tool_use_id = %v, want call_agent_123", got)
	}
	if got := toolResult["name"]; got != "Agent" {
		t.Fatalf("user tool result name = %v, want Agent", got)
	}
	if got := toolResult["content"]; got != "Exploration complete." {
		t.Fatalf("user tool result content = %v, want Exploration complete.", got)
	}
}
