package copilot

import (
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
)

// selectShape returns the copilotAPIShape to use for the given (already
// remapped) model name and incoming request.
//
// Priority order:
//  1. ForceChatCompletions extension → shapeChat
//  2. Model found in shapeCache → use cached shape
//  3. Cache miss / nil cache → fall back to IsGPT5Family heuristic
func (a *CopilotAdapter) selectShape(model string, request *cif.CanonicalRequest) copilotAPIShape {
	// Normalize once; use the same key for cache lookup and heuristic.
	normalized := strings.ToLower(strings.TrimSpace(model))
	if isKnownResponsesOnlyModelKey(normalized) {
		return shapeResponses
	}
	if a.forceChatCompletions(request) {
		return shapeChat
	}

	if shape, ok := a.provider.lookupShape(normalized); ok {
		return shape
	}

	// Cache miss: fall back to family heuristics so the provider works
	// before GetModels() has been called.
	if shared.IsGPT5Family(normalized) && !strings.Contains(normalized, "-mini") {
		return shapeResponses
	}
	return shapeChat
}

func isKnownResponsesOnlyModelKey(model string) bool {
	return model == "grok-4.5" || model == "grok-4.6"
}
