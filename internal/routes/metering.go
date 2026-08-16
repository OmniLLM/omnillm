package routes

import (
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"strings"
	"time"
)

// recordUsage writes a metering row asynchronously after a completed request.
// It is called from the four response handlers (streaming + non-streaming,
// OpenAI + Anthropic shapes).  Errors are logged but never returned — metering
// must never break the request path.
func normalizeMeteringClient(userAgent string) string {
	client := strings.TrimSpace(userAgent)
	if client == "" {
		return "unknown"
	}
	if parts := strings.Fields(client); len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

func recordUsage(
	requestID string,
	modelID string, // canonical model as the caller requested
	modelUsed string, // actual model reported by the provider
	providerID string,
	client string,
	apiShape string,
	usage *cif.CIFUsage,
	latencyMS int64,
	isStream bool,
	statusCode int,
	errMsg string,
) {
	inputTokens := 0
	var uncachedInputTokens *int
	var cacheReadInputTokens *int
	var cacheWriteInputTokens *int
	var cacheWrite5mInputTokens *int
	var cacheWrite1hInputTokens *int
	outputTokens := 0
	if usage != nil {
		inputTokens = usage.InputTokens
		uncachedInputTokens = usage.UncachedInputTokens
		cacheReadInputTokens = usage.CacheReadInputTokens
		cacheWriteInputTokens = usage.CacheWriteInputTokens
		cacheWrite5mInputTokens = usage.CacheWrite5mInputTokens
		cacheWrite1hInputTokens = usage.CacheWrite1hInputTokens
		outputTokens = usage.OutputTokens
	}

	rec := database.MeteringRecord{
		RequestID:               requestID,
		ModelID:                 modelID,
		ModelUsed:               modelUsed,
		ProviderID:              providerID,
		Client:                  client,
		APIShape:                apiShape,
		InputTokens:             inputTokens,
		UncachedInputTokens:     uncachedInputTokens,
		CacheReadInputTokens:    cacheReadInputTokens,
		CacheWriteInputTokens:   cacheWriteInputTokens,
		CacheWrite5mInputTokens: cacheWrite5mInputTokens,
		CacheWrite1hInputTokens: cacheWrite1hInputTokens,
		OutputTokens:            outputTokens,
		TotalTokens:             inputTokens + outputTokens,
		LatencyMS:               latencyMS,
		IsStream:                isStream,
		StatusCode:              statusCode,
		ErrorMessage:            errMsg,
		CreatedAt:               time.Now().UTC(),
	}

	database.EnqueueMeteringRecord(rec)
}
