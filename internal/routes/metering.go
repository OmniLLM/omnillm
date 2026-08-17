package routes

import (
	"net/http"
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
	rec := newMeteringRecord(requestID, modelID, modelUsed, providerID, client, apiShape, usage, latencyMS, isStream, statusCode, errMsg)
	if usage != nil {
		rec.UncachedInputTokens = usage.UncachedInputTokens
		rec.CacheReadInputTokens = usage.CacheReadInputTokens
		rec.CacheWriteInputTokens = usage.CacheWriteInputTokens
		rec.CacheWrite5mInputTokens = usage.CacheWrite5mInputTokens
		rec.CacheWrite1hInputTokens = usage.CacheWrite1hInputTokens
	}
	database.EnqueueMeteringRecord(rec)
}

func newMeteringRecord(
	requestID string,
	modelID string,
	modelUsed string,
	providerID string,
	client string,
	apiShape string,
	usage *cif.CIFUsage,
	latencyMS int64,
	isStream bool,
	statusCode int,
	errMsg string,
) database.MeteringRecord {
	inputTokens := 0
	outputTokens := 0
	if usage != nil {
		inputTokens = usage.InputTokens
		outputTokens = usage.OutputTokens
	}
	return database.MeteringRecord{
		RequestID:    requestID,
		ModelID:      modelID,
		ModelUsed:    modelUsed,
		ProviderID:   providerID,
		Client:       client,
		APIShape:     apiShape,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		LatencyMS:    latencyMS,
		IsStream:     isStream,
		StatusCode:   statusCode,
		ErrorMessage: errMsg,
		CreatedAt:    time.Now().UTC(),
	}
}

// recordResponseCacheHit records aggregate served volume for an exact-response
// cache hit without copying historical provider prompt-cache counters from the
// cached payload. No provider call occurred, so those counters must stay nil.
func recordResponseCacheHit(
	requestID string,
	modelID string,
	modelUsed string,
	client string,
	apiShape string,
	usage *cif.CIFUsage,
	latencyMS int64,
	isStream bool,
) {
	rec := newMeteringRecord(
		requestID, modelID, modelUsed, "response-cache", client, apiShape,
		usage, latencyMS, isStream, http.StatusOK, "",
	)
	database.EnqueueMeteringRecord(rec)
}
