package routes

import (
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
	"omnillm/internal/lib/affinity"
	"omnillm/internal/lib/approval"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/lib/ratelimit"
	"omnillm/internal/lib/responsecache"
	"omnillm/internal/providerdispatch"
	"omnillm/internal/providers/types"
	"omnillm/internal/serialization"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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

	// Exact-match response cache (opt-in, deterministic requests).
	cacheCfg := responsecache.LoadConfig()
	bypass := responsecache.ParseBypass(c.GetHeader(responsecache.BypassHeader))
	var cacheKey string
	cacheEligible := cacheCfg.Enabled && bypass != responsecache.BypassAll && responsecache.Cacheable(canonicalRequest)
	if cacheEligible {
		cacheKey = responsecache.Key(canonicalRequest)
		if bypass != responsecache.BypassRead {
			if hit := responsecache.Get(cacheCfg, canonicalRequest, cacheKey); hit != nil {
				if canonicalRequest.Stream {
					replayOpenAIStreamFromCache(c, hit)
					logCompletedResponse("openai", requestIDStr, originalModel, hit.Model, "cache", true, hit.StopReason, hit.Usage, startTime)
					return
				}
				openaiResp, err := serialization.SerializeToOpenAI(hit)
				if err == nil {
					c.Header("X-OmniLLM-Cache", "hit")
					logCompletedResponse("openai", requestIDStr, originalModel, hit.Model, "cache", false, hit.StopReason, hit.Usage, startTime)
					c.JSON(http.StatusOK, openaiResp)
					return
				}
				log.Warn().Err(err).Str("request_id", requestIDStr).Msg("Cache hit failed to serialize; falling through to upstream")
			}
		}
	}

	if cacheEligible {
		c.Set("responsecache_key", cacheKey)
	}

	// Resolve providers for the requested model
	attempts := resolveRequestedModels(requestIDStr, canonicalRequest.Model)
	executor := providerdispatch.NewExecutor(providerdispatch.ApplyGitHubCopilotSingleUpstreamMode, providerdispatch.DefaultUpstreamAPI)
	resolveFailed := false
	lastErr := executor.TryAttempts(
		toDispatchAttempts(attempts),
		canonicalRequest,
		modelCache,
		modelrouting.ResolveProvidersForModel,
		func(attempt providerdispatch.Attempt) {
			log.Warn().Str("request_id", requestIDStr).Str("model", attempt.RequestedModel).Msg("No providers available for model attempt")
		},
		func(attempt providerdispatch.Attempt, err error) {
			resolveFailed = true
			log.Error().Err(err).Str("request_id", requestIDStr).Str("model", attempt.RequestedModel).Msg("Failed to resolve providers for model")
		},
		func(candidate *providerdispatch.Candidate, providerID string) error {
			log.Debug().
				Str("request_id", requestIDStr).
				Str("model", candidate.CanonicalModel).
				Str("provider", providerID).
				Msg("Trying provider for request")

			if candidate.UpstreamModel != candidate.CanonicalModel {
				log.Debug().Str("request_id", requestIDStr).Str("from", candidate.CanonicalModel).Str("to", candidate.UpstreamModel).Msg("Remapped model name")
			}

			var err error
			if candidate.Request.Stream {
				err = handleStreamingResponse(c, candidate.Adapter, candidate.Request, requestIDStr, originalModel, providerID, startTime)
			} else {
				err = handleNonStreamingResponse(c, candidate.Adapter, candidate.Request, requestIDStr, originalModel, providerID, startTime)
			}
			if err != nil {
				log.Warn().Err(err).
					Str("request_id", requestIDStr).
					Str("provider", providerID).
					Str("upstream_model", candidate.UpstreamModel).
					Msg("Provider failed, trying next")
			} else {
				affinity.Get().Record(canonicalRequest, candidate.CanonicalModel, providerID)
			}
			return err
		},
	)
	if lastErr == nil {
		return
	}

	if resolveFailed {
		writeResolveProvidersError(c, lastErr, "provider_error")
		return
	}
	writeProviderFailure(c, "provider_error", lastErr)
}

//nolint:dupl // structurally similar to responses.go but serves different API shape (chat vs responses)
func handleNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
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
	recordUsage(requestID, originalModel, response.Model, providerID, normalizeMeteringClient(c.GetHeader("User-Agent")), "openai", response.Usage, time.Since(startTime).Milliseconds(), false, http.StatusOK, "")

	// Populate the exact-match response cache if this request was eligible.
	if key, ok := c.Get("responsecache_key"); ok {
		if keyStr, _ := key.(string); keyStr != "" {
			responsecache.Put(responsecache.LoadConfig(), canonicalRequest, keyStr, response)
			c.Header("X-OmniLLM-Cache", "miss")
		}
	}

	c.JSON(http.StatusOK, openaiResp)
	return nil
}

func handleStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
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

	// If this request is cache-eligible, accumulate the stream into a
	// CanonicalResponse so it can be stored and replayed on a future hit.
	var acc *responsecache.StreamAccumulator
	var cacheKey string
	if key, ok := c.Get("responsecache_key"); ok {
		if ks, _ := key.(string); ks != "" {
			cacheKey = ks
			acc = responsecache.NewStreamAccumulator()
			c.Header("X-OmniLLM-Cache", "miss")
		}
	}

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

			if acc != nil {
				acc.Observe(event)
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
				recordUsage(requestID, originalModel, modelUsed, providerID, normalizeMeteringClient(c.GetHeader("User-Agent")), "openai", endEvt.Usage, time.Since(startTime).Milliseconds(), true, http.StatusOK, "")

				if acc != nil {
					if assembled := acc.Response(); assembled != nil {
						responsecache.Put(responsecache.LoadConfig(), canonicalRequest, cacheKey, assembled)
					}
				}
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

// replayOpenAIStreamFromCache re-emits a cached CanonicalResponse as an OpenAI
// SSE stream, so a streaming client gets a cache hit as a normal (if instant)
// stream. It reuses the production event serializer via synthesized CIF events.
func replayOpenAIStreamFromCache(c *gin.Context, resp *cif.CanonicalResponse) {
	c.Header("X-OmniLLM-Cache", "hit")
	setSSEHeaders(c, true)
	state := serialization.CreateOpenAIStreamState()
	flusher, _ := c.Writer.(http.Flusher)
	events := responsecache.SynthesizeStream(resp)
	c.Stream(func(w io.Writer) bool {
		for _, ev := range events {
			sseData, err := serialization.ConvertCIFEventToOpenAISSE(ev, state)
			if err != nil {
				return false
			}
			if sseData != "" {
				flushStreamWriter(w, flusher, sseData)
			}
		}
		return false
	})
}
