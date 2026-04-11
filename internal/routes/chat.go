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
	"omnimodel/internal/lib/approval"
	"omnimodel/internal/lib/modelrouting"
	"omnimodel/internal/lib/ratelimit"
	"omnimodel/internal/providers/types"
	"omnimodel/internal/serialization"
)

var (
	modelCache     = make(modelrouting.ModelCache)
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
		Msg("--> REQUEST")

	// Resolve providers for the requested model
	resolvedModel, normalizedModel := resolveRequestedModel(requestIDStr, canonicalRequest.Model)
	canonicalRequest.Model = resolvedModel

	modelRoute, err := modelrouting.ResolveProvidersForModel(
		canonicalRequest.Model,
		normalizedModel,
		modelCache,
	)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Str("model", canonicalRequest.Model).Msg("Failed to resolve providers for model")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Failed to resolve providers: %v", err),
				"type":    "provider_error",
			},
		})
		return
	}

	if len(modelRoute.CandidateProviders) == 0 {
		log.Warn().Str("request_id", requestIDStr).Str("model", canonicalRequest.Model).Msg("No providers available for model")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' not found or no providers available", canonicalRequest.Model),
				"type":    "model_not_found",
			},
		})
		return
	}

	if normalizedModel != canonicalRequest.Model {
		log.Debug().Str("request_id", requestIDStr).Str("from", canonicalRequest.Model).Str("to", normalizedModel).Msg("Normalized chat request model")
		canonicalRequest.Model = normalizedModel
	}

	// Try candidate providers in priority order
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
			Msg("Trying provider for request")

		// Remap model name for this provider
		remappedModel := adapter.RemapModel(canonicalRequest.Model)
		if remappedModel != canonicalRequest.Model {
			log.Debug().Str("request_id", requestIDStr).Str("from", canonicalRequest.Model).Str("to", remappedModel).Msg("Remapped model name")
			canonicalRequest.Model = remappedModel
		}

		if canonicalRequest.Stream {
			lastErr = handleStreamingResponse(c, adapter, canonicalRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
		} else {
			lastErr = handleNonStreamingResponse(c, adapter, canonicalRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
		}

		if lastErr == nil {
			return // success
		}

		log.Warn().Err(lastErr).
			Str("request_id", requestIDStr).
			Str("provider", provider.GetInstanceID()).
			Msg("Provider failed, trying next")
	}

	// All providers failed
	errMsg := "All providers failed"
	if lastErr != nil {
		errMsg = fmt.Sprintf("All providers failed. Last error: %v", lastErr)
	}
	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"message": errMsg,
			"type":    "provider_error",
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
		Msg("<-- RESPONSE")

	c.JSON(http.StatusOK, openaiResp)
	return nil
}

func handleStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(canonicalRequest)
	if err != nil {
		// Fallback to non-streaming if streaming not supported
		log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming not supported, falling back to non-streaming")
		canonicalRequest.Stream = false
		return handleNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	state := serialization.CreateOpenAIStreamState()
	flusher, _ := c.Writer.(http.Flusher)
	modelUsed := canonicalRequest.Model

	c.Stream(func(w io.Writer) bool {
		event, ok := <-eventCh
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
				Msg("<-- RESPONSE stream")
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

// writeSSE writes a single SSE event to the response writer
func writeSSE(c *gin.Context, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.Writer, "data: %s\n\n", string(jsonBytes))
	return err
}
