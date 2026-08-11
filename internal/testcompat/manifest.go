package testcompat

import providertypes "omnillm/internal/providers/types"

type UpstreamShape string

type StreamingStrategy string

const (
	ShapeChat      UpstreamShape = "chat_completions"
	ShapeResponses UpstreamShape = "responses"
	ShapeClaude    UpstreamShape = "claude"
	ShapeGemini    UpstreamShape = "gemini"

	StreamNative      StreamingStrategy = "native"
	StreamSynthesized StreamingStrategy = "synthesized"
)

type Capability struct {
	Supported           bool
	NotApplicableReason string
}

type ProviderStrategy struct {
	Name              string
	Provider          providertypes.ProviderID
	Model             string
	UpstreamShape     UpstreamShape
	StreamingStrategy StreamingStrategy
	Tools             Capability
	Streaming         Capability
}

func ProviderStrategies() []ProviderStrategy {
	return []ProviderStrategy{
		strategy("copilot-chat", providertypes.ProviderGitHubCopilot, "claude-opus-4.7", ShapeChat, StreamNative),
		strategy("copilot-responses", providertypes.ProviderGitHubCopilot, "gpt-5.5", ShapeResponses, StreamNative),
		strategy("copilot-claude-stream", providertypes.ProviderGitHubCopilot, "claude-opus-4.7", ShapeClaude, StreamNative),
		strategy("antigravity-gemini", providertypes.ProviderAntigravity, "gemini-3-pro-high", ShapeGemini, StreamNative),
		strategy("alibaba-chat", providertypes.ProviderAlibaba, "qwen3.6-plus", ShapeChat, StreamNative),
		strategy("modelscope-chat", providertypes.ProviderModelScope, "Qwen/Qwen3-Coder-Next", ShapeChat, StreamNative),
		strategy("azure-responses", providertypes.ProviderAzureOpenAI, "gpt-5.4", ShapeResponses, StreamNative),
		strategy("google-gemini", providertypes.ProviderGoogle, "gemini-3-pro-preview", ShapeGemini, StreamNative),
		strategy("kimi-chat", providertypes.ProviderKimi, "kimi-k2.5", ShapeChat, StreamNative),
		strategy("codex-api-key-chat", providertypes.ProviderCodex, "gpt-5.4", ShapeChat, StreamNative),
		strategy("openai-oauth-responses", providertypes.ProviderOpenAI, "gpt-5.4", ShapeResponses, StreamNative),
		strategy("openai-compatible-chat", providertypes.ProviderOpenAICompatible, "compat-chat-model", ShapeChat, StreamNative),
		strategy("openai-compatible-responses", providertypes.ProviderOpenAICompatible, "compat-responses-model", ShapeResponses, StreamNative),
	}
}

func SupportedProviderTypes() []providertypes.ProviderID {
	return providertypes.SupportedProviderIDs()
}

func strategy(name string, provider providertypes.ProviderID, model string, shape UpstreamShape, streaming StreamingStrategy) ProviderStrategy {
	return ProviderStrategy{
		Name: name, Provider: provider, Model: model, UpstreamShape: shape, StreamingStrategy: streaming,
		Tools: Capability{Supported: true}, Streaming: Capability{Supported: true},
	}
}
