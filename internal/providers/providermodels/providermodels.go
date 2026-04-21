// Package providermodels is the single source of truth for upstream API shape
// selection.  Every provider adapter must consult UpstreamAPI() instead of
// maintaining its own ad-hoc heuristics.
//
// Design mirrors LiteLLM's model_prices_and_context_window approach: a
// declarative table of provider → upstream API shape that adapters consume.
// Routing policy lives here; adapters stay thin.
//
// # API shapes
//
//   - "chat.completions"  — OpenAI-compatible /chat/completions (default)
//   - "responses"         — OpenAI Responses API /responses
//
// # Mapping (provider-level, not per-model)
//
//	github-copilot    → chat.completions  (Copilot exposes the chat completions API)
//	azure-openai      → responses         (Azure OpenAI uses the Responses API)
//	alibaba           → chat.completions
//	kimi              → chat.completions
//	openai-compatible → chat.completions  (default; operator may override via api_format config)
package providermodels

import "strings"

// APIShape is the upstream wire protocol to use for a provider.
type APIShape string

const (
	// ChatCompletions is the OpenAI-compatible /chat/completions protocol.
	ChatCompletions APIShape = "chat.completions"

	// Responses is the OpenAI Responses API /responses protocol.
	Responses APIShape = "responses"
)

// table maps provider ID → upstream API shape.
// Provider IDs match types.ProviderID constants.
// Unlisted providers default to ChatCompletions.
var table = map[string]APIShape{
	// Copilot exposes the OpenAI chat completions API for all models.
	"github-copilot": ChatCompletions,

	// Azure OpenAI uses the Responses API for all models.
	"azure-openai": Responses,

	// DashScope speaks the OpenAI-compatible chat completions protocol.
	"alibaba": ChatCompletions,

	"kimi": ChatCompletions,

	// Generic user-configured endpoints default to chat completions.
	// Operators may override per-instance via api_format config.
	"openai-compatible": ChatCompletions,
}

// UpstreamAPI returns the upstream API shape for the given provider.
// model is accepted for forward-compatibility but is not currently used.
func UpstreamAPI(providerID, _ string) APIShape {
	if shape, ok := table[strings.TrimSpace(providerID)]; ok {
		return shape
	}
	return ChatCompletions
}

// IsChatCompletions is a convenience predicate.
func IsChatCompletions(providerID, model string) bool {
	return UpstreamAPI(providerID, model) == ChatCompletions
}

// IsResponses is a convenience predicate.
func IsResponses(providerID, model string) bool {
	return UpstreamAPI(providerID, model) == Responses
}
