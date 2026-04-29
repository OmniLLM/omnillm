package routes

import (
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
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
	outputTokens := 0
	if usage != nil {
		inputTokens = usage.InputTokens
		outputTokens = usage.OutputTokens
	}

	rec := database.MeteringRecord{
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

	go func() {
		db := database.GetDatabase()
		if err := db.InsertMeteringRecord(rec); err != nil {
			log.Error().Err(err).
				Str("request_id", requestID).
				Msg("Failed to record metering data")
		}
	}()
}
