package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/providers/types"
	"omnillm/internal/serialization"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func SetupResponseRoutes(router *gin.RouterGroup) {
	router.POST("/responses", handleResponses)
}

func handleResponses(c *gin.Context) {
	// Type assertion is zero-allocation vs fmt.Sprintf("%v", requestID)
	requestID, _ := c.Get("request_id")
	requestIDStr, _ := requestID.(string)
	startTime := time.Now()

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to read request body")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Convert Responses API format to CIF.
	// json.Valid is omitted: ParseResponsesPayload calls json.Unmarshal which
	// already validates syntax and returns a clear error, avoiding a double parse pass.
	canonicalRequest, err := ingestion.ParseResponsesPayload(body)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to parse Responses API request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": parseRequestMessage(err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	originalModel := prepareCanonicalRequest(c, canonicalRequest, "responses")

	// Resolve providers
	resolvedModel, normalizedModel := resolveRequestedModel(requestIDStr, canonicalRequest.Model)
	canonicalRequest.Model = resolvedModel
	modelRoute, err := modelrouting.ResolveProvidersForModel(
		canonicalRequest.Model,
		normalizedModel,
		modelCache,
	)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Str("model", canonicalRequest.Model).Msg("Failed to resolve providers")
		writeResolveProvidersError(c, err, "server_error")
		return
	}

	if len(modelRoute.CandidateProviders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' not found or no providers available", canonicalRequest.Model),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if normalizedModel != canonicalRequest.Model {
		log.Debug().
			Str("request_id", requestIDStr).
			Str("from", canonicalRequest.Model).
			Str("to", normalizedModel).
			Msg("Normalized Responses API request model")
		canonicalRequest.Model = normalizedModel
	}

	// Try candidate providers
	var lastErr error
	for _, provider := range modelRoute.CandidateProviders {
		adapter := provider.GetAdapter()
		if adapter == nil {
			continue
		}

		providerRequest := *canonicalRequest

		log.Debug().
			Str("request_id", requestIDStr).
			Str("model", providerRequest.Model).
			Str("provider", provider.GetInstanceID()).
			Msg("Trying provider for Responses API request")

		remappedModel := adapter.RemapModel(providerRequest.Model)
		log.Debug().
			Str("request_id", requestIDStr).
			Str("provider", provider.GetInstanceID()).
			Str("api_shape", "responses").
			Str("inbound_path", c.FullPath()).
			Str("upstream_api", detectUpstreamAPI(provider.GetID(), adapter, &providerRequest, remappedModel)).
			Str("canonical_model", providerRequest.Model).
			Str("upstream_model", remappedModel).
			Msg("Converted CIF request to upstream model API")
		providerRequest.Model = remappedModel

		if providerRequest.Stream {
			lastErr = handleResponsesStreamingResponse(c, adapter, &providerRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
		} else {
			lastErr = handleResponsesNonStreamingResponse(c, adapter, &providerRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
		}

		if lastErr == nil {
			return
		}

		log.Warn().Err(lastErr).
			Str("request_id", requestIDStr).
			Str("provider", provider.GetInstanceID()).
			Str("upstream_model", providerRequest.Model).
			Msg("Provider failed for Responses API request, trying next")
	}

	writeProviderFailure(c, "server_error", lastErr)
}

func handleResponsesNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
	response, err := adapter.Execute(c.Request.Context(), canonicalRequest)
	if err != nil {
		return fmt.Errorf("adapter execute failed: %w", err)
	}

	responsesResp, err := serialization.SerializeToResponses(response)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}

	logCompletedResponse("responses", requestID, originalModel, response.Model, providerID, false, response.StopReason, response.Usage, startTime)

	c.JSON(http.StatusOK, responsesResp)
	return nil
}

func handleResponsesStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(c.Request.Context(), canonicalRequest)
	if err != nil {
		if shouldFallbackToNonStreaming(err) {
			log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming request failed before stream start, retrying as non-streaming")
			canonicalRequest.Stream = false
			return handleResponsesNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
		}
		return err
	}

	setSSEHeaders(c, false)

	state := serialization.CreateResponsesStreamState()
	flusher, _ := c.Writer.(http.Flusher)
	modelUsed := canonicalRequest.Model
	ctx := c.Request.Context()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case event, ok := <-eventCh:
			if !ok {
				return false
			}

			responsesEvents, err := serialization.ConvertCIFEventToResponsesSSE(event, state)
			if err != nil {
				log.Error().Err(err).Str("request_id", requestID).Msg("Failed to convert CIF event to Responses SSE")
				return false
			}

			var sb strings.Builder
			for _, evt := range responsesEvents {
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
			io.WriteString(w, sb.String())

			if flusher != nil {
				flusher.Flush()
			}

			if endEvt, isEnd := event.(cif.CIFStreamEnd); isEnd {
				inputTokens := 0
				outputTokens := 0
				if endEvt.Usage != nil {
					inputTokens = endEvt.Usage.InputTokens
					outputTokens = endEvt.Usage.OutputTokens
				}

				log.Info().
					Str("request_id", requestID).
					Str("api_shape", "responses").
					Str("model_requested", originalModel).
					Str("model_used", modelUsed).
					Str("provider", providerID).
					Str("stop_reason", string(endEvt.StopReason)).
					Bool("stream", true).
					Int("input_tokens", inputTokens).
					Int("output_tokens", outputTokens).
					Int64("latency_ms", time.Since(startTime).Milliseconds()).
					Msg("\x1b[32m<--\x1b[0m RESPONSE stream")
				return false
			}

			if _, isErr := event.(cif.CIFStreamError); isErr {
				return false
			}

			return true
		}
	})

	return nil
}
