package providermodels_test

import (
	"omnillm/internal/providers/providermodels"
	"testing"
)

func TestUpstreamAPI(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     providermodels.APIShape
	}{
		// GitHub Copilot — always chat completions
		{"github-copilot", "gpt-4o", providermodels.ChatCompletions},
		{"github-copilot", "claude-3.5-sonnet", providermodels.ChatCompletions},
		{"github-copilot", "claude-haiku-4.5", providermodels.ChatCompletions},
		{"github-copilot", "claude-sonnet-4.6", providermodels.ChatCompletions},
		{"github-copilot", "gpt-5-mini", providermodels.ChatCompletions},
		{"github-copilot", "gpt-5.4", providermodels.ChatCompletions},
		{"github-copilot", "gpt-5-codex", providermodels.ChatCompletions},

		// Azure OpenAI — always responses
		{"azure-openai", "gpt-4o", providermodels.Responses},
		{"azure-openai", "gpt-4o-mini", providermodels.Responses},
		{"azure-openai", "gpt-5.4", providermodels.Responses},

		// Alibaba — always chat completions
		{"alibaba", "qwen3.6-plus", providermodels.ChatCompletions},
		{"alibaba", "deepseek-v4-flash", providermodels.ChatCompletions},
		{"alibaba", "qwq-32b", providermodels.ChatCompletions},

		// Kimi — always chat completions
		{"kimi", "moonshot-v1-8k", providermodels.ChatCompletions},

		// openai-compatible — default chat completions
		{"openai-compatible", "llama3", providermodels.ChatCompletions},
		{"openai-compatible", "mistral-7b", providermodels.ChatCompletions},

		// Unknown provider — falls back to chat completions
		{"unknown-provider", "some-model", providermodels.ChatCompletions},
	}

	for _, tc := range tests {
		got := providermodels.UpstreamAPI(tc.provider, tc.model)
		if got != tc.want {
			t.Errorf("UpstreamAPI(%q, %q) = %q, want %q", tc.provider, tc.model, got, tc.want)
		}
	}
}

func TestPredicates(t *testing.T) {
	if !providermodels.IsChatCompletions("alibaba", "qwen3.6-plus") {
		t.Error("IsChatCompletions(alibaba, qwen3.6-plus) should be true")
	}
	if !providermodels.IsChatCompletions("github-copilot", "gpt-4o") {
		t.Error("IsChatCompletions(github-copilot, gpt-4o) should be true")
	}
	if !providermodels.IsResponses("azure-openai", "gpt-4o") {
		t.Error("IsResponses(azure-openai, gpt-4o) should be true")
	}
	if providermodels.IsResponses("github-copilot", "gpt-5.4") {
		t.Error("IsResponses(github-copilot, gpt-5.4) should be false — copilot is chat completions")
	}
}
