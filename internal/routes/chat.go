package routes

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnimodel/internal/cif"
	"omnimodel/internal/ingestion"
	"omnimodel/internal/lib/approval"
	"omnimodel/internal/lib/modelrouting"
	"omnimodel/internal/lib/ratelimit"
	"omnimodel/internal/providers/types"
	"omnimodel/internal/serialization"
)

var (
	modelCache     = modelrouting.NewModelCache()
	rateLimiter    *ratelimit.RateLimiter
	manualApproval bool
)

func SetupChatCompletionRoutes(router *gin.RouterGroup) {
	rateLimiter = ratelimit.NewRateLimiter(0, false)
	manualApproval = false

	router.POST("/chat/completions", handleChatCompletions)
}

func ConfigureChatCompletionOptions(rl *ratelimit.RateLimiter, manual bool) {
	if rl != nil {
		rateLimiter = rl
	}
	manualApproval = manual
}

func handleChatCompletions(c *gin.Context) {
	requestID, _ := c.Get("request_id")
	requestIDStr := fmt.Sprintf("%v", requestID)
	startTime := time.Now()

	// Check rate limits
	if err := rateLimiter.CheckAndWait(); err != nil {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "rate_limit_exceeded",
			},
		})
		return
	}

	// Manual approval if enabled
	if manualApproval {
		if err := approval.AwaitApproval(); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"message": err.Error(),
					"type":    "request_rejected",
				},
			})
			return
		}
	}

	// Parse request as generic map for ingestion
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to parse request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Convert to CIF
	canonicalRequest, err := ingestion.ParseOpenAIChatCompletions(payload)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to parse OpenAI request")
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
		Str("api_shape", "openai").
		Str("model_requested", originalModel).
		Int("messages", len(canonicalRequest.Messages)).
		Int("tools", len(canonicalRequest.Tools)).
		Bool("stream", canonicalRequest.Stream).
		Msg("\x1b[33m-->\x1b[0m REQUEST")

	// Resolve providers for the requested model
	attempts := resolveRequestedModels(requestIDStr, canonicalRequest.Model)

	var lastErr error
	for _, attempt := range attempts {
		attemptRequest := *canonicalRequest
		attemptRequest.Model = attempt.RequestedModel

		modelRoute, err := modelrouting.ResolveProvidersForModel(
			attempt.RequestedModel,
			attempt.NormalizedModel,
			modelCache,
		)
		if err != nil {
			log.Error().Err(err).Str("request_id", requestIDStr).Str("model", attempt.RequestedModel).Msg("Failed to resolve providers for model")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"message": fmt.Sprintf("Failed to resolve providers: %v", err),
					"type":    "provider_error",
				},
			})
			return
		}

		if len(modelRoute.CandidateProviders) == 0 {
			lastErr = fmt.Errorf("model '%s' not found or no providers available", attempt.RequestedModel)
			log.Warn().Str("request_id", requestIDStr).Str("model", attempt.RequestedModel).Msg("No providers available for model attempt")
			continue
		}

		if attempt.NormalizedModel != attemptRequest.Model {
			log.Debug().Str("request_id", requestIDStr).Str("from", attemptRequest.Model).Str("to", attempt.NormalizedModel).Msg("Normalized chat request model")
			attemptRequest.Model = attempt.NormalizedModel
		}

		// Try candidate providers in priority order
		for _, provider := range modelRoute.CandidateProviders {
			adapter := provider.GetAdapter()
			if adapter == nil {
				continue
			}

			providerRequest := attemptRequest
			applyGitHubCopilotSingleUpstreamMode(provider, &providerRequest)

			log.Debug().
				Str("request_id", requestIDStr).
				Str("model", providerRequest.Model).
				Str("provider", provider.GetInstanceID()).
				Msg("Trying provider for request")

			// Remap model name for this provider
			remappedModel := adapter.RemapModel(providerRequest.Model)
			if remappedModel != providerRequest.Model {
				log.Debug().Str("request_id", requestIDStr).Str("from", providerRequest.Model).Str("to", remappedModel).Msg("Remapped model name")
				providerRequest.Model = remappedModel
			}

			if providerRequest.Stream {
				lastErr = handleStreamingResponse(c, adapter, &providerRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
			} else {
				lastErr = handleNonStreamingResponse(c, adapter, &providerRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
			}

			if lastErr == nil {
				return // success
			}

			log.Warn().Err(lastErr).
				Str("request_id", requestIDStr).
				Str("provider", provider.GetInstanceID()).
				Str("upstream_model", providerRequest.Model).
				Msg("Provider failed, trying next")
		}
	}

	// All providers failed
	errMsg := "All providers failed"
	if lastErr != nil {
		errMsg = fmt.Sprintf("All providers failed. Last error: %v", lastErr)
	}
	c.JSON(providerFailureStatus(lastErr), gin.H{
		"error": gin.H{
			"message": errMsg,
			"type":    providerFailureType("provider_error", lastErr),
		},
	})
}

func handleNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	response, err := adapter.Execute(canonicalRequest)
	if err != nil {
		return fmt.Errorf("adapter execute failed: %w", err)
	}

	// Serialize CIF response to OpenAI format
	openaiResp, err := serialization.SerializeToOpenAI(response)
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
		Str("api_shape", "openai").
		Str("model_requested", originalModel).
		Str("model_used", response.Model).
		Str("provider", providerID).
		Str("stop_reason", string(response.StopReason)).
		Bool("stream", false).
		Int("input_tokens", inputTokens).
		Int("output_tokens", outputTokens).
		Int64("latency_ms", time.Since(startTime).Milliseconds()).
		Msg("\x1b[32m<--\x1b[0m RESPONSE")

	c.JSON(http.StatusOK, openaiResp)
	return nil
}

func handleStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(canonicalRequest)
	if err != nil {
		if shouldFallbackToNonStreaming(err) && allowStreamingFallback(canonicalRequest) {
			log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming request failed before stream start, retrying as non-streaming")
			canonicalRequest.Stream = false
			return handleNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
		}
		return err
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Wrap upstream channel so it exits when client disconnects.
	ctx := c.Request.Context()
	wrappedCh := make(chan cif.CIFStreamEvent, cap(eventCh))
	go func() {
		defer close(wrappedCh)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				wrappedCh <- evt
			}
		}
	}()

	state := serialization.CreateOpenAIStreamState()
	flusher, _ := c.Writer.(http.Flusher)
	modelUsed := canonicalRequest.Model

	c.Stream(func(w io.Writer) bool {
		event, ok := <-wrappedCh
		if !ok {
			return false
		}

		sseData, err := serialization.ConvertCIFEventToOpenAISSE(event, state)
		if err != nil {
			log.Error().Err(err).Str("request_id", requestID).Msg("Failed to convert CIF event to SSE")
			return false
		}

		if sseData != "" {
			fmt.Fprint(w, sseData)
			if flusher != nil {
				flusher.Flush()
			}
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
				Str("api_shape", "openai").
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

		// Check if this is the end event
		if _, isErr := event.(cif.CIFStreamError); isErr {
			return false
		}

		return true
	})

	return nil
}
