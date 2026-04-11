package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnimodel/internal/cif"
	"omnimodel/internal/ingestion"
	"omnimodel/internal/lib/modelrouting"
	"omnimodel/internal/providers/types"
	"omnimodel/internal/serialization"
)

func SetupResponseRoutes(router *gin.RouterGroup) {
	router.POST("/responses", handleResponses)
}

func handleResponses(c *gin.Context) {
	requestID, _ := c.Get("request_id")
	requestIDStr := fmt.Sprintf("%v", requestID)
	startTime := time.Now()

	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Convert Responses API format to CIF
	canonicalRequest, err := ingestion.ParseResponsesPayload(payload)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to parse Responses API request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Failed to parse request: %v", err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	originalModel := canonicalRequest.Model

	// Log REQUEST
	log.Info().
		Str("request_id", requestIDStr).
		Str("api_shape", "responses").
		Str("model_requested", originalModel).
		Int("messages", len(canonicalRequest.Messages)).
		Int("tools", len(canonicalRequest.Tools)).
		Bool("stream", canonicalRequest.Stream).
		Msg("--> REQUEST")

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
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Failed to resolve providers: %v", err),
				"type":    "server_error",
			},
		})
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

		log.Debug().
			Str("request_id", requestIDStr).
			Str("model", canonicalRequest.Model).
			Str("provider", provider.GetInstanceID()).
			Msg("Trying provider for Responses API request")

		remappedModel := adapter.RemapModel(canonicalRequest.Model)
		log.Debug().
			Str("request_id", requestIDStr).
			Str("provider", provider.GetInstanceID()).
			Str("api_shape", "responses").
			Str("inbound_path", c.FullPath()).
			Str("upstream_api", upstreamAPIForProvider(provider.GetID(), remappedModel)).
			Str("canonical_model", canonicalRequest.Model).
			Str("upstream_model", remappedModel).
			Msg("Converted CIF request to upstream model API")
		canonicalRequest.Model = remappedModel

		if canonicalRequest.Stream {
			lastErr = handleResponsesStreamingResponse(c, adapter, canonicalRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
		} else {
			lastErr = handleResponsesNonStreamingResponse(c, adapter, canonicalRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
		}

		if lastErr == nil {
			return
		}

		log.Warn().Err(lastErr).
			Str("request_id", requestIDStr).
			Str("provider", provider.GetInstanceID()).
			Msg("Provider failed for Responses API request, trying next")
	}

	errMsg := "All providers failed"
	if lastErr != nil {
		errMsg = fmt.Sprintf("All providers failed. Last error: %v", lastErr)
	}
	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"message": errMsg,
			"type":    "server_error",
		},
	})
}

func handleResponsesNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	response, err := adapter.Execute(canonicalRequest)
	if err != nil {
		return fmt.Errorf("adapter execute failed: %w", err)
	}

	responsesResp, err := serialization.SerializeToResponses(response)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}

	inputTokens := 0
	outputTokens := 0
	if response.Usage != nil {
		inputTokens = response.Usage.InputTokens
		outputTokens = response.Usage.OutputTokens
	}

	log.Info().
		Str("request_id", requestID).
		Str("api_shape", "responses").
		Str("model_requested", originalModel).
		Str("model_used", response.Model).
		Str("provider", providerID).
		Str("stop_reason", string(response.StopReason)).
		Bool("stream", false).
		Int("input_tokens", inputTokens).
		Int("output_tokens", outputTokens).
		Int64("latency_ms", time.Since(startTime).Milliseconds()).
		Msg("<-- RESPONSE")

	c.JSON(http.StatusOK, responsesResp)
	return nil
}

func handleResponsesStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(canonicalRequest)
	if err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming not supported, falling back to non-streaming")
		canonicalRequest.Stream = false
		return handleResponsesNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	state := serialization.CreateResponsesStreamState()
	flusher, _ := c.Writer.(http.Flusher)
	modelUsed := canonicalRequest.Model

	c.Stream(func(w io.Writer) bool {
		event, ok := <-eventCh
		if !ok {
			return false
		}

		responsesEvents, err := serialization.ConvertCIFEventToResponsesSSE(event, state)
		if err != nil {
			log.Error().Err(err).Str("request_id", requestID).Msg("Failed to convert CIF event to Responses SSE")
			return false
		}

		for _, evt := range responsesEvents {
			eventType, _ := evt["type"].(string)
			jsonBytes, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(jsonBytes))
		}

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
				Msg("<-- RESPONSE stream")
			return false
		}

		if _, isErr := event.(cif.CIFStreamError); isErr {
			return false
		}

		return true
	})

	return nil
}
