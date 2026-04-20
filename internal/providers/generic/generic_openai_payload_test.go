package generic

import (
	"testing"

	"omnillm/internal/cif"
)

func TestBuildOpenAIPayloadIncludesSystemPrompt(t *testing.T) {
	provider := NewGenericProvider("azure-openai", "azure-openai-test", "Azure OpenAI")
	adapter := &GenericAdapter{provider: provider}

	systemPrompt := "Be concise."
	request := &cif.CanonicalRequest{
		Model:        "gpt-4o",
		SystemPrompt: &systemPrompt,
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "hello"},
				},
			},
		},
	}

	payload := adapter.buildOpenAIPayload(request)
	messages, ok := payload["messages"].([]map[string]interface{})
	if !ok {
		t.Fatalf("messages payload has unexpected type %T", payload["messages"])
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0]["role"] != "system" {
		t.Fatalf("expected first role to be system, got %v", messages[0]["role"])
	}
	if messages[0]["content"] != systemPrompt {
		t.Fatalf("expected system prompt %q, got %v", systemPrompt, messages[0]["content"])
	}
	if messages[1]["role"] != "user" {
		t.Fatalf("expected second role to be user, got %v", messages[1]["role"])
	}
}

func TestAlibabaBuildOpenAIPayloadPreservesQwen36Plus(t *testing.T) {
	provider := NewGenericProvider("alibaba", "alibaba-test", "Alibaba")
	adapter := &GenericAdapter{provider: provider}

	payload := adapter.buildOpenAIPayload(&cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "hello"},
				},
			},
		},
	})

	if payload["model"] != "qwen3.6-plus" {
		t.Fatalf("expected preserved model qwen3.6-plus, got %v", payload["model"])
	}
}

// TestAlibabaReasoningModelEnableThinking verifies that enable_thinking is set
// for a Qwen3 reasoning model without tools.
func TestAlibabaReasoningModelEnableThinking(t *testing.T) {
	provider := NewGenericProvider("alibaba", "alibaba-test", "Alibaba")
	adapter := &GenericAdapter{provider: provider}

	payload := adapter.buildOpenAIPayload(&cif.CanonicalRequest{
		Model: "qwen3-max",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "think"}},
			},
		},
	})

	if val, ok := payload["enable_thinking"]; !ok || val != true {
		t.Errorf("expected enable_thinking=true for reasoning model, got %v (present=%v)", val, ok)
	}
}

// TestAlibabaGlobalNonReasoningModelNoEnableThinking verifies that enable_thinking
// is NOT set for a non-reasoning model.
func TestAlibabaGlobalNonReasoningModelNoEnableThinking(t *testing.T) {
	provider := NewGenericProvider("alibaba", "alibaba-test", "Alibaba")
	adapter := &GenericAdapter{provider: provider}

	payload := adapter.buildOpenAIPayload(&cif.CanonicalRequest{
		Model: "qwen2-5-72b-instruct", // not a reasoning model
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hello"}},
			},
		},
	})

	if val, ok := payload["enable_thinking"]; ok {
		t.Errorf("expected enable_thinking to be absent for non-reasoning model, got %v", val)
	}
}

// ─── Qwen3.6-Plus Tool Use Tests ──────────────────────────────────────────────

func TestQwen36PlusToolUseWithRealTools(t *testing.T) {
	provider := NewGenericProvider("alibaba", "qwen36-test", "Qwen 3.6 Plus")
	adapter := &GenericAdapter{provider: provider}

	desc := "Get the current weather for a location"
	payload := adapter.buildOpenAIPayload(&cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "What's the weather in New York?"},
				},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:        "get_weather",
				Description: &desc,
				ParametersSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{"type": "string"},
					},
					"required": []string{"location"},
				},
			},
		},
	})

	tools, ok := payload["tools"].([]map[string]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("expected tools in payload")
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	fn, ok := tools[0]["function"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tool.function to be map, got %T", tools[0]["function"])
	}
	if fn["name"] != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %v", fn["name"])
	}
	// tool_choice should NOT be forced to "none" when real tools are present
	if toolChoice, ok := payload["tool_choice"]; ok && toolChoice == "none" {
		t.Errorf("tool_choice should not be 'none' when real tools are provided, got %v", toolChoice)
	}
}

func TestQwen36PlusToolUseWithToolChoice(t *testing.T) {
	provider := NewGenericProvider("alibaba", "qwen36-test", "Qwen 3.6 Plus")
	adapter := &GenericAdapter{provider: provider}

	desc := "Make an API call"
	toolChoice := "required"
	payload := adapter.buildOpenAIPayload(&cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Call the API"},
				},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:        "api_call",
				Description: &desc,
				ParametersSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"endpoint": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
		ToolChoice: toolChoice,
	})

	if tc, ok := payload["tool_choice"]; !ok || tc != toolChoice {
		t.Errorf("expected tool_choice=%q, got %v (ok=%v)", toolChoice, tc, ok)
	}
}

// TestQwen36PlusNoToolsNoEnableThinking verifies that qwen3.6-plus without tools
// gets enable_thinking set (it is a reasoning model), but NOT a dummy tool.
func TestQwen36PlusNoToolsInjectsEnableThinking(t *testing.T) {
	provider := NewGenericProvider("alibaba", "qwen36-test", "Qwen 3.6 Plus")
	adapter := &GenericAdapter{provider: provider}

	payload := adapter.buildOpenAIPayload(&cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Hello"},
				},
			},
		},
	})

	// enable_thinking should be set for reasoning model with no tools
	if val, ok := payload["enable_thinking"]; !ok || val != true {
		t.Errorf("expected enable_thinking=true for reasoning model without tools, got %v (present=%v)", val, ok)
	}
	// No tools should be injected
	if tools, ok := payload["tools"]; ok && tools != nil {
		t.Errorf("expected no tools for request without tools, got %v", tools)
	}
}
