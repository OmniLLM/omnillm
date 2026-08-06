package alibaba

import (
	"context"
	"strings"
)

func RemapModel(modelID string) string {
	return strings.TrimSpace(modelID)
}

func IsChatCompletionsModel(modelID string) bool {
	return !strings.Contains(strings.ToLower(modelID), "realtime")
}

// IsReasoningModel reports whether the model supports extended reasoning output.
// Qwen 3.6 Plus has a provider-owned fallback because its DashScope contract
// must not depend on availability of the optional models.dev metadata service.
func IsReasoningModel(ctx context.Context, modelID string) bool {
	return isReasoningModelWith(ctx, defaultModelMetadataLookup, modelID)
}

// isReasoningModelWith is the testable core: the caller supplies the lookup so
// tests can inject a stub without needing a live models.dev connection.
func isReasoningModelWith(ctx context.Context, lookup modelMetadataLookup, modelID string) bool {
	modelID = strings.ToLower(RemapModel(modelID))
	if modelID == "qwen3.6-plus" {
		return true
	}
	meta := lookup(ctx, modelID)
	if meta != nil && meta.SupportsReasoning != nil {
		return *meta.SupportsReasoning
	}
	return false
}
