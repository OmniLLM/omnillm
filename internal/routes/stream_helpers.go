package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func setSSEHeaders(c *gin.Context, chunked bool) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	if chunked {
		c.Header("Transfer-Encoding", "chunked")
	}
}

func flushStreamWriter(w io.Writer, flusher http.Flusher, data string) {
	if data == "" {
		return
	}
	_, _ = io.WriteString(w, data)
	if flusher != nil {
		flusher.Flush()
	}
}

func usageTokenCounts(usage *cif.CIFUsage) (int, int) {
	if usage == nil {
		return 0, 0
	}
	return usage.InputTokens, usage.OutputTokens
}

func logCompletedResponse(apiShape, requestID, originalModel, modelUsed, providerID string, stream bool, stopReason cif.CIFStopReason, usage *cif.CIFUsage, startTime time.Time) {
	inputTokens, outputTokens := usageTokenCounts(usage)
	message := "\x1b[32m<--\x1b[0m RESPONSE"
	if stream {
		message += " stream"
	}

	log.Info().
		Str("request_id", requestID).
		Str("api_shape", apiShape).
		Str("model_requested", originalModel).
		Str("model_used", modelUsed).
		Str("provider", providerID).
		Str("stop_reason", string(stopReason)).
		Bool("stream", stream).
		Int("input_tokens", inputTokens).
		Int("output_tokens", outputTokens).
		Int64("latency_ms", time.Since(startTime).Milliseconds()).
		Msg(message)
}

func formatResponsesSSE(events []map[string]interface{}) string {
	if len(events) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, evt := range events {
		eventType, _ := evt["type"].(string)
		jsonBytes, err := json.Marshal(evt)
		if err != nil {
			continue
		}
		sb.WriteString("event: ")
		sb.WriteString(eventType)
		sb.WriteString("\ndata: ")
		sb.Write(jsonBytes)
		sb.WriteString("\n\n")
	}
	return sb.String()
}
