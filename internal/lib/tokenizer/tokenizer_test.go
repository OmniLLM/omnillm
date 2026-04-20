package tokenizer

import (
	"omnillm/internal/providers/types"
	"testing"
)

// ─── GetTokenizerFromModel ───

func TestGetTokenizerFromModel_GPT(t *testing.T) {
	m := &types.Model{ID: "gpt-4o"}
	tok := GetTokenizerFromModel(m)
	if tok != "o200k_base" {
		t.Errorf("gpt-4o: expected 'o200k_base', got %q", tok)
	}
}

func TestGetTokenizerFromModel_GPT35(t *testing.T) {
	m := &types.Model{ID: "gpt-3.5-turbo"}
	tok := GetTokenizerFromModel(m)
	if tok != "o200k_base" {
		t.Errorf("gpt-3.5-turbo: expected 'o200k_base', got %q", tok)
	}
}

func TestGetTokenizerFromModel_Claude(t *testing.T) {
	m := &types.Model{ID: "claude-3-5-sonnet-20241022"}
	tok := GetTokenizerFromModel(m)
	if tok != "claude" {
		t.Errorf("claude model: expected 'claude', got %q", tok)
	}
}

func TestGetTokenizerFromModel_Gemini(t *testing.T) {
	m := &types.Model{ID: "gemini-2.0-flash-001"}
	tok := GetTokenizerFromModel(m)
	if tok != "gemini" {
		t.Errorf("gemini model: expected 'gemini', got %q", tok)
	}
}

func TestGetTokenizerFromModel_Unknown(t *testing.T) {
	m := &types.Model{ID: "unknown-model-xyz"}
	tok := GetTokenizerFromModel(m)
	if tok != "o200k_base" {
		t.Errorf("unknown model should fallback to 'o200k_base', got %q", tok)
	}
}

func TestGetTokenizerFromModel_FromCapabilities(t *testing.T) {
	m := &types.Model{
		ID: "some-model",
		Capabilities: map[string]interface{}{
			"tokenizer": "claude",
		},
	}
	tok := GetTokenizerFromModel(m)
	if tok != "claude" {
		t.Errorf("capability tokenizer: expected 'claude', got %q", tok)
	}
}

// ─── EstimateTokenCount ───

func TestEstimateTokenCount_SimpleText(t *testing.T) {
	payload := &ChatCompletionsPayload{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "Hello, world!"},
		},
	}
	model := &types.Model{ID: "gpt-4o"}
	count, err := EstimateTokenCount(payload, model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count.Input <= 0 {
		t.Errorf("expected positive input token count, got %d", count.Input)
	}
	// "Hello, world!" is 13 chars → ~3-4 tokens, plus message overhead
	if count.Input > 20 {
		t.Errorf("input tokens seem too high for short text: %d", count.Input)
	}
}

func TestEstimateTokenCount_AssistantTokensAreSeparate(t *testing.T) {
	payload := &ChatCompletionsPayload{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "What is the weather?"},
			{Role: "assistant", Content: "It is sunny today."},
		},
	}
	model := &types.Model{ID: "gpt-4o"}
	count, err := EstimateTokenCount(payload, model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count.Input <= 0 {
		t.Error("expected positive input tokens")
	}
	if count.Output <= 0 {
		t.Error("expected positive output tokens for assistant message")
	}
}

func TestEstimateTokenCount_WithTools(t *testing.T) {
	payload := &ChatCompletionsPayload{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "Get weather"},
		},
		Tools: []Tool{
			{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_weather",
					Description: "Returns current weather for a location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}
	model := &types.Model{ID: "gpt-4o"}
	countWithTools, err := EstimateTokenCount(payload, model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Token count with tools should be higher than without
	payloadNoTools := &ChatCompletionsPayload{
		Model:    "gpt-4o",
		Messages: payload.Messages,
	}
	countNoTools, _ := EstimateTokenCount(payloadNoTools, model)

	if countWithTools.Input <= countNoTools.Input {
		t.Errorf("tools should add tokens: with=%d, without=%d",
			countWithTools.Input, countNoTools.Input)
	}
}

func TestEstimateTokenCount_ClaudeTokenizer(t *testing.T) {
	payload := &ChatCompletionsPayload{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []Message{
			{Role: "user", Content: "Hello world"},
		},
	}
	model := &types.Model{ID: "claude-3-5-sonnet-20241022"}
	count, err := EstimateTokenCount(payload, model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count.Input <= 0 {
		t.Error("expected positive input tokens for Claude model")
	}
}

func TestEstimateTokenCount_EmptyMessages(t *testing.T) {
	payload := &ChatCompletionsPayload{
		Model:    "gpt-4o",
		Messages: []Message{},
	}
	model := &types.Model{ID: "gpt-4o"}
	count, err := EstimateTokenCount(payload, model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty messages should return 0 or near-zero tokens
	if count.Input < 0 || count.Output < 0 {
		t.Errorf("token counts should not be negative: input=%d, output=%d",
			count.Input, count.Output)
	}
}
