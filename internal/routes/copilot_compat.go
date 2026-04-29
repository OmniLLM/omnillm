package routes

import (
	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
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
	if !shared.IsGPT5Family(request.Model) {
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
