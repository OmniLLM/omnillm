package routes

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
	"omnillm/internal/lib/approval"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/lib/ratelimit"
	"omnillm/internal/providers/types"
	"omnillm/internal/serialization"
)

var modelCache = modelrouting.NewModelCache()

type ChatCompletionOptions struct {
	RateLimiter    *ratelimit.RateLimiter
	ManualApproval bool
}

type chatCompletionHandler struct {
	rateLimiter    *ratelimit.RateLimiter
	manualApproval bool
}

func SetupChatCompletionRoutes(router *gin.RouterGroup, options ChatCompletionOptions) {
	handler := newChatCompletionHandler(options)
	router.POST("/chat/completions", handler.handleChatCompletions)
}

func newChatCompletionHandler(options ChatCompletionOptions) *chatCompletionHandler {
	rl := options.RateLimiter
	if rl == nil {
		rl = ratelimit.NewRateLimiter(0, false)
	}

	return &chatCompletionHandler{
		rateLimiter:    rl,
		manualApproval: options.ManualApproval,
	}
}

func (h *chatCompletionHandler) handleChatCompletions(c *gin.Context) {
	// Type assertion is zero-allocation vs fmt.Sprintf("%v", requestID)
	requestID, _ := c.Get("request_id")
	requestIDStr, _ := requestID.(string)
	startTime := time.Now()

	// Check rate limits
	if err := h.rateLimiter.CheckAndWait(); err != nil {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "rate_limit_exceeded",
			},
		})
		return
	}

	// Manual approval if enabled
	if h.manualApproval {
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

	// Parse request body and convert to CIF.
	// json.Valid is omitted: ParseOpenAIChatCompletions calls json.Unmarshal which
	// already validates syntax and returns a clear error, avoiding a double parse pass.
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

	canonicalRequest, err := ingestion.ParseOpenAIChatCompletions(body)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to parse OpenAI request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": parseRequestMessage(err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	originalModel := prepareCanonicalRequest(c, canonicalRequest, "openai")

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
			writeResolveProvidersError(c, err, "provider_error")
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

	writeProviderFailure(c, "provider_error", lastErr)
}

func handleNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	response, err := adapter.Execute(c.Request.Context(), canonicalRequest)
	if err != nil {
		return fmt.Errorf("adapter execute failed: %w", err)
	}

	// Serialize CIF response to OpenAI format
	openaiResp, err := serialization.SerializeToOpenAI(response)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}

	logCompletedResponse("openai", requestID, originalModel, response.Model, providerID, false, response.StopReason, response.Usage, startTime)

	c.JSON(http.StatusOK, openaiResp)
	return nil
}

func handleStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(c.Request.Context(), canonicalRequest)
	if err != nil {
		if shouldFallbackToNonStreaming(err) && allowStreamingFallback(canonicalRequest) {
			log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming request failed before stream start, retrying as non-streaming")
			canonicalRequest.Stream = false
			return handleNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
		}
		return err
	}

	setSSEHeaders(c, true)

	state := serialization.CreateOpenAIStreamState()
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

			sseData, err := serialization.ConvertCIFEventToOpenAISSE(event, state)
			if err != nil {
				log.Error().Err(err).Str("request_id", requestID).Msg("Failed to convert CIF event to SSE")
				return false
			}

			if sseData != "" {
				flushStreamWriter(w, flusher, sseData)
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

			if _, isErr := event.(cif.CIFStreamError); isErr {
				return false
			}

			return true
		}
	})

	return nil
}
