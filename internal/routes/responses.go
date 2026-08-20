package routes

import (
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
	"omnillm/internal/lib/affinity"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/lib/responsecache"
	"omnillm/internal/providerdispatch"
	"omnillm/internal/providers/types"
	"omnillm/internal/serialization"
	"omnillm/internal/translation/toolarguments"
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

	body, err := readGatewayRequestBody(c.Request.Body)
	if err != nil {
		status, message := gatewayRequestBodyError(err)
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to read request body")
		c.JSON(status, gin.H{
			"error": gin.H{
				"message": message,
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

	if hit := lookupResponseCache(c, canonicalRequest, requestIDStr); hit != nil {
		if canonicalRequest.Stream {
			if replayResponsesStreamFromCache(c, hit) {
				logCompletedResponse("responses", requestIDStr, originalModel, hit.Model, "response-cache", true, hit.StopReason, hit.Usage, startTime)
				recordResponseCacheHit(requestIDStr, originalModel, hit.Model, normalizeMeteringClient(c.GetHeader("User-Agent")), "responses", hit.Usage, time.Since(startTime).Milliseconds(), true)
				return
			}
			log.Warn().Str("request_id", requestIDStr).Msg("Cache hit failed to serialize; falling through to upstream")
		} else {
			responsesResp, err := serialization.SerializeToResponses(hit)
			if err == nil {
				c.Header(responsecache.BypassHeader, "hit")
				logCompletedResponse("responses", requestIDStr, originalModel, hit.Model, "response-cache", false, hit.StopReason, hit.Usage, startTime)
				recordResponseCacheHit(requestIDStr, originalModel, hit.Model, normalizeMeteringClient(c.GetHeader("User-Agent")), "responses", hit.Usage, time.Since(startTime).Milliseconds(), false)
				c.JSON(http.StatusOK, responsesResp)
				return
			}
			log.Warn().Err(err).Str("request_id", requestIDStr).Msg("Cache hit failed to serialize; falling through to upstream")
		}
	}

	// Resolve providers using the same ordered attempt pipeline as the other
	// generation dialects so provider pinning and failover remain intact.
	attempts, err := resolveRequestedModelsForRequest(requestIDStr, canonicalRequest.Model, canonicalRequest)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_provider_reference"}})
		return
	}
	executor := providerdispatch.NewExecutor(providerdispatch.ApplyGitHubCopilotSingleUpstreamMode, providerdispatch.DefaultUpstreamAPI)
	resolveFailed := false
	lastErr := executor.TryAttempts(
		toDispatchAttempts(attempts),
		canonicalRequest,
		modelCache,
		modelrouting.ResolveProvidersForModel,
		nil,
		func(attempt providerdispatch.Attempt, err error) {
			resolveFailed = true
			log.Error().Err(err).Str("request_id", requestIDStr).Str("model", attempt.RequestedModel).Msg("Failed to resolve providers")
		},
		func(candidate *providerdispatch.Candidate, providerID string) error {
			log.Debug().
				Str("request_id", requestIDStr).
				Str("model", candidate.CanonicalModel).
				Str("provider", providerID).
				Msg("Trying provider for Responses API request")
			recordAttemptedCandidate(c, providerID, candidate.UpstreamModel)

			log.Debug().
				Str("request_id", requestIDStr).
				Str("provider", providerID).
				Str("api_shape", "responses").
				Str("inbound_path", c.FullPath()).
				Str("upstream_api", candidate.UpstreamAPI).
				Str("canonical_model", candidate.CanonicalModel).
				Str("upstream_model", candidate.UpstreamModel).
				Msg("Converted CIF request to upstream model API")

			var err error
			if candidate.Request.Stream {
				err = handleResponsesStreamingResponse(c, candidate.Adapter, candidate.Request, requestIDStr, originalModel, providerID, startTime)
			} else {
				err = handleResponsesNonStreamingResponse(c, candidate.Adapter, candidate.Request, requestIDStr, originalModel, providerID, startTime)
			}
			if err != nil {
				if isClientCanceled(c, err) {
					log.Info().
						Str("request_id", requestIDStr).
						Str("provider", providerID).
						Str("upstream_model", candidate.UpstreamModel).
						Msg("Client canceled request, abandoning failover")
					return providerdispatch.Abort(err)
				}
				log.Warn().Err(err).
					Str("request_id", requestIDStr).
					Str("provider", providerID).
					Str("upstream_model", candidate.UpstreamModel).
					Msg("Provider failed for Responses API request, trying next")
			}
			if err == nil {
				affinity.Get().Record(canonicalRequest, candidate.CanonicalModel, providerID)
			}
			return err
		},
	)
	if lastErr == nil {
		return
	}
	if resolveFailed {
		writeResolveProvidersError(c, lastErr, "server_error")
		return
	}
	if strings.Contains(lastErr.Error(), "not found or no providers available") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": lastErr.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}
	writeProviderFailure(c, "server_error", lastErr)
}

//nolint:dupl // structurally similar to chat.go but serves different API shape (responses vs chat)
func handleResponsesNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
	response, err := adapter.Execute(c.Request.Context(), canonicalRequest)
	if err != nil {
		return fmt.Errorf("adapter execute failed: %w", err)
	}

	response = toolarguments.NormalizeResponse(response, canonicalRequest.Tools)

	responsesResp, err := serialization.SerializeToResponses(response)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}

	logCompletedResponse("responses", requestID, originalModel, response.Model, providerID, false, response.StopReason, response.Usage, startTime)
	recordUsage(requestID, originalModel, response.Model, providerID, normalizeMeteringClient(c.GetHeader("User-Agent")), "responses", response.Usage, time.Since(startTime).Milliseconds(), false, http.StatusOK, "")

	populateResponseCache(c, canonicalRequest, response)

	c.JSON(http.StatusOK, responsesResp)
	return nil
}

func handleResponsesStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(c.Request.Context(), canonicalRequest)
	if err != nil {
		if !isClientCanceled(c, err) && shouldFallbackToNonStreaming(err) {
			log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming request failed before stream start, retrying as non-streaming")
			canonicalRequest.Stream = false
			return handleResponsesNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
		}
		return err
	}

	setSSEHeaders(c, false)
	ctx := c.Request.Context()
	eventCh = toolarguments.NormalizeStream(ctx, eventCh, canonicalRequest.Tools)

	acc, cacheState := newResponseCacheStreamAccumulator(c)

	state := serialization.CreateResponsesStreamState()
	flusher, _ := c.Writer.(http.Flusher)
	modelUsed := canonicalRequest.Model

	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case event, ok := <-eventCh:
			if !ok {
				return false
			}

			if acc != nil {
				acc.Observe(event)
			}

			responsesEvents, err := serialization.ConvertCIFEventToResponsesSSE(event, state)
			if err != nil {
				log.Error().Err(err).Str("request_id", requestID).Msg("Failed to convert CIF event to Responses SSE")
				return false
			}

			flushStreamWriter(w, flusher, formatResponsesSSE(responsesEvents))

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
				recordUsage(requestID, originalModel, modelUsed, providerID, normalizeMeteringClient(c.GetHeader("User-Agent")), "responses", endEvt.Usage, time.Since(startTime).Milliseconds(), true, http.StatusOK, "")
				populateResponseCacheStream(c, canonicalRequest, acc, cacheState)
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

func replayResponsesStreamFromCache(c *gin.Context, response *cif.CanonicalResponse) bool {
	state := serialization.CreateResponsesStreamState()
	var payloads []string
	for _, event := range responsecache.SynthesizeStream(response) {
		responsesEvents, err := serialization.ConvertCIFEventToResponsesSSE(event, state)
		if err != nil {
			return false
		}
		if payload := formatResponsesSSE(responsesEvents); payload != "" {
			payloads = append(payloads, payload)
		}
	}

	return replayCachedSSE(c, payloads, false)
}
