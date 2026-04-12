package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnimodel/internal/cif"
	"omnimodel/internal/ingestion"
	"omnimodel/internal/lib/modelrouting"
	"omnimodel/internal/providers/types"
	"omnimodel/internal/serialization"
)

func SetupMessageRoutes(router *gin.RouterGroup) {
	router.POST("/messages", handleMessages)
	router.POST("/messages/count_tokens", handleCountTokens)
}

func upstreamAPIForProvider(providerID string, model string) string {
	if providerID == "azure-openai" && strings.Contains(strings.ToLower(model), "gpt-5.4") {
		return "responses"
	}
	return "chat.completions"
}

func handleMessages(c *gin.Context) {
	requestID, _ := c.Get("request_id")
	requestIDStr := fmt.Sprintf("%v", requestID)
	startTime := time.Now()

	// Parse request as generic map for ingestion
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

	// Convert Anthropic format to CIF
	canonicalRequest, err := ingestion.ParseAnthropicMessages(payload)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to parse Anthropic request")
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
		Str("api_shape", "anthropic").
		Str("model_requested", originalModel).
		Int("messages", len(canonicalRequest.Messages)).
		Int("tools", len(canonicalRequest.Tools)).
		Bool("stream", canonicalRequest.Stream).
		Msg("\x1b[33m-->\x1b[0m REQUEST")

	// Resolve providers
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
			log.Error().Err(err).Str("request_id", requestIDStr).Str("model", attempt.RequestedModel).Msg("Failed to resolve providers")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"message": fmt.Sprintf("Failed to resolve providers: %v", err),
					"type":    "api_error",
				},
			})
			return
		}

		if len(modelRoute.CandidateProviders) == 0 {
			lastErr = fmt.Errorf("model '%s' not found or no providers available", attempt.RequestedModel)
			continue
		}

		if attempt.NormalizedModel != attemptRequest.Model {
			log.Debug().
				Str("request_id", requestIDStr).
				Str("from", attemptRequest.Model).
				Str("to", attempt.NormalizedModel).
				Msg("Normalized Anthropic request model")
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
				Msg("Trying provider for Anthropic request")

			remappedModel := adapter.RemapModel(providerRequest.Model)
			log.Debug().
				Str("request_id", requestIDStr).
				Str("provider", provider.GetInstanceID()).
				Str("api_shape", "anthropic").
				Str("inbound_path", c.FullPath()).
				Str("upstream_api", upstreamAPIForProvider(provider.GetID(), remappedModel)).
				Str("canonical_model", providerRequest.Model).
				Str("upstream_model", remappedModel).
				Msg("Converted CIF request to upstream model API")
			providerRequest.Model = remappedModel

			if providerRequest.Stream {
				lastErr = handleAnthropicStreamingResponse(c, adapter, &providerRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
			} else {
				lastErr = handleAnthropicNonStreamingResponse(c, adapter, &providerRequest, requestIDStr, originalModel, provider.GetInstanceID(), startTime)
			}

			if lastErr == nil {
				return
			}

			log.Warn().Err(lastErr).
				Str("request_id", requestIDStr).
				Str("provider", provider.GetInstanceID()).
				Str("upstream_model", attempt.RequestedModel).
				Msg("Provider failed for Anthropic request, trying next")
		}
	}

	errMsg := "All providers failed"
	if lastErr != nil {
		errMsg = fmt.Sprintf("All providers failed. Last error: %v", lastErr)
	}
	c.JSON(providerFailureStatus(lastErr), gin.H{
		"error": gin.H{
			"message": errMsg,
			"type":    providerFailureType("api_error", lastErr),
		},
	})
}

func handleAnthropicNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	response, err := adapter.Execute(canonicalRequest)
	if err != nil {
		return fmt.Errorf("adapter execute failed: %w", err)
	}

	anthropicResp, err := serialization.SerializeToAnthropic(response)
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
		Str("api_shape", "anthropic").
		Str("model_requested", originalModel).
		Str("model_used", response.Model).
		Str("provider", providerID).
		Str("stop_reason", string(response.StopReason)).
		Bool("stream", false).
		Int("input_tokens", inputTokens).
		Int("output_tokens", outputTokens).
		Int64("latency_ms", time.Since(startTime).Milliseconds()).
		Msg("\x1b[32m<--\x1b[0m RESPONSE")

	c.JSON(http.StatusOK, anthropicResp)
	return nil
}

func handleAnthropicStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID string, originalModel string, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(canonicalRequest)
	if err != nil {
		if shouldFallbackToNonStreaming(err) && allowStreamingFallback(canonicalRequest) {
			log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming request failed before stream start, retrying as non-streaming")
			canonicalRequest.Stream = false
			return handleAnthropicNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
		}
		return err
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	state := serialization.CreateAnthropicStreamState()
	flusher, _ := c.Writer.(http.Flusher)
	modelUsed := canonicalRequest.Model

	c.Stream(func(w io.Writer) bool {
		event, ok := <-eventCh
		if !ok {
			return false
		}

		anthropicEvents, err := serialization.ConvertCIFEventToAnthropicSSE(event, state)
		if err != nil {
			log.Error().Err(err).Str("request_id", requestID).Msg("Failed to convert CIF event to Anthropic SSE")
			return false
		}

		for _, evt := range anthropicEvents {
			eventType, _ := evt["type"].(string)
			sseData, err := serialization.FormatAnthropicSSEData(eventType, evt)
			if err != nil {
				continue
			}
			fmt.Fprint(w, sseData)
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
				Str("api_shape", "anthropic").
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
	})

	return nil
}

func handleCountTokens(c *gin.Context) {
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

	// Parse the Anthropic request to count tokens
	canonicalRequest, err := ingestion.ParseAnthropicMessages(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Failed to parse request: %v", err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Estimate token count from messages
	totalTokens := 0
	for _, msg := range canonicalRequest.Messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			totalTokens += estimateStringTokens(m.Content)
		case cif.CIFUserMessage:
			for _, part := range m.Content {
				if tp, ok := part.(cif.CIFTextPart); ok {
					totalTokens += estimateStringTokens(tp.Text)
				}
			}
		case cif.CIFAssistantMessage:
			for _, part := range m.Content {
				if tp, ok := part.(cif.CIFTextPart); ok {
					totalTokens += estimateStringTokens(tp.Text)
				}
			}
		}
	}

	// Add tool tokens if present
	for _, tool := range canonicalRequest.Tools {
		totalTokens += estimateStringTokens(tool.Name)
		if tool.Description != nil {
			totalTokens += estimateStringTokens(*tool.Description)
		}
		if tool.ParametersSchema != nil {
			schemaBytes, _ := json.Marshal(tool.ParametersSchema)
			totalTokens += estimateStringTokens(string(schemaBytes))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"input_tokens": totalTokens,
	})
}

// estimateStringTokens provides a rough character-based token estimate
func estimateStringTokens(s string) int {
	// ~4 characters per token is a reasonable average for English
	tokens := len(s) / 4
	if tokens == 0 && len(s) > 0 {
		tokens = 1
	}
	return tokens
}
