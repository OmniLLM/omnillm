package generic

import (
	"testing"

	"omnimodel/internal/cif"
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

func TestAlibabaOAuthBuildOpenAIPayloadInjectsQwenSystemMessage(t *testing.T) {
	provider := NewGenericProvider("alibaba", "alibaba-oauth-test", "Alibaba")
	provider.applyConfig(map[string]interface{}{
		"auth_type":    "oauth",
		"resource_url": "portal.qwen.ai",
	})
	adapter := &GenericAdapter{provider: provider}

	request := &cif.CanonicalRequest{
		Model: "qwen3-coder-flash",
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
	if messages[0]["content"] != "You are Qwen Code." {
		t.Fatalf("expected Qwen system prompt, got %v", messages[0]["content"])
	}
}

func TestAlibabaOAuthBuildOpenAIPayloadMergesSystemPrompts(t *testing.T) {
	provider := NewGenericProvider("alibaba", "alibaba-oauth-test", "Alibaba")
	provider.applyConfig(map[string]interface{}{
		"auth_type":    "oauth",
		"resource_url": "portal.qwen.ai",
	})
	adapter := &GenericAdapter{provider: provider}

	systemPrompt := "Be concise."
	request := &cif.CanonicalRequest{
		Model:        "qwen3-coder-flash",
		SystemPrompt: &systemPrompt,
		Messages: []cif.CIFMessage{
			cif.CIFSystemMessage{
				Role:    "system",
				Content: "Use Markdown when helpful.",
			},
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
		t.Fatalf("expected exactly one merged system message plus the user message, got %d", len(messages))
	}
	if messages[0]["role"] != "system" {
		t.Fatalf("expected first role to be system, got %v", messages[0]["role"])
	}

	want := "You are Qwen Code.\n\nBe concise.\n\nUse Markdown when helpful."
	if messages[0]["content"] != want {
		t.Fatalf("expected merged system prompt %q, got %v", want, messages[0]["content"])
	}
}
