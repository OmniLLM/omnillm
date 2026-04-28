package routes

import (
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/providers/types"
)

func applyGitHubCopilotSingleUpstreamMode(provider types.Provider, request *cif.CanonicalRequest) {
	if provider == nil || request == nil || provider.GetID() != string(types.ProviderGitHubCopilot) {
		return
	}

	if request.Extensions == nil {
		request.Extensions = &cif.Extensions{}
	}

	trueValue := true
	// Copilot GPT-5 family models are Responses-only upstream. Do not force
	// chat completions for those models, or the provider can never switch to
	// /responses and upstream returns `unsupported_api_for_model`.
	model := strings.ToLower(strings.TrimSpace(request.Model))
	if !strings.HasPrefix(model, "gpt-5") {
		request.Extensions.ForceChatCompletions = &trueValue
	}
	request.Extensions.DisableAuthRetry = &trueValue
	request.Extensions.DisableStreamingFallback = &trueValue
}

func allowStreamingFallback(request *cif.CanonicalRequest) bool {
	if request == nil || request.Extensions == nil || request.Extensions.DisableStreamingFallback == nil {
		return true
	}

	return !*request.Extensions.DisableStreamingFallback
}
