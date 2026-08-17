package routes

import (
	"encoding/json"
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

	"github.com/rs/zerolog"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func SetupMessageRoutes(router *gin.RouterGroup) {
	router.POST("/messages", handleMessages)
	router.POST("/messages/count_tokens", handleCountTokens)
}

type alibabaModeProvider interface {
	AlibabaAPIMode() string
}

var upstreamAPIForProvider = providerdispatch.DefaultUpstreamAPI

func logLatencyProbe(requestID, stage string, started time.Time) {
	if log.Logger.GetLevel() > zerolog.DebugLevel {
		return
	}
	log.Debug().
		Str("request_id", requestID).
		Str("stage", stage).
		Int64("elapsed_ms", time.Since(started).Milliseconds()).
		Msg("Latency probe")
}

func handleMessages(c *gin.Context) {
	// Type assertion is zero-allocation vs fmt.Sprintf("%v", requestID)
	requestID, _ := c.Get("request_id")
	requestIDStr, _ := requestID.(string)
	startTime := time.Now()

	// Parse request body and convert to CIF.
	// json.Valid is omitted: ParseAnthropicMessages calls json.Unmarshal which
	// already validates syntax and returns a clear error, avoiding a double parse pass.
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

	// Convert Anthropic format to CIF first, then extract the map for tool loop
	// logging from the structured request — avoids a second json.Unmarshal pass.
	canonicalRequest, err := ingestion.ParseAnthropicMessages(body)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to parse Anthropic request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": parseRequestMessage(err),
				"type":    "invalid_request_error",
			},
		})
		return
	}
	logLatencyProbe(requestIDStr, "anthropic_parse", startTime)

	// Parse into map for tool loop logging (lazy: only if logger is active)
	var payloadMap map[string]interface{}
	_ = json.Unmarshal(body, &payloadMap)
	logRawAnthropicToolLoopPayload(requestIDStr, payloadMap)

	originalModel := prepareCanonicalRequest(c, canonicalRequest, "anthropic")
	logAnthropicToolLoopRequest(requestIDStr, canonicalRequest)

	if hit := lookupResponseCache(c, canonicalRequest, requestIDStr); hit != nil {
		suppressThinking := !strings.Contains(c.GetHeader("anthropic-beta"), "interleaved-thinking")
		if canonicalRequest.Stream {
			if replayAnthropicStreamFromCache(c, hit, suppressThinking) {
				logCompletedResponse("anthropic", requestIDStr, originalModel, hit.Model, "response-cache", true, hit.StopReason, hit.Usage, startTime)
				recordResponseCacheHit(requestIDStr, originalModel, hit.Model, normalizeMeteringClient(c.GetHeader("User-Agent")), "anthropic", hit.Usage, time.Since(startTime).Milliseconds(), true)
				return
			}
			log.Warn().Str("request_id", requestIDStr).Msg("Cache hit failed to serialize; falling through to upstream")
		} else {
			anthropicResp, err := serialization.SerializeToAnthropicWithSuppression(hit, suppressThinking)
			if err == nil {
				c.Header(responsecache.BypassHeader, "hit")
				logCompletedResponse("anthropic", requestIDStr, originalModel, hit.Model, "response-cache", false, hit.StopReason, hit.Usage, startTime)
				recordResponseCacheHit(requestIDStr, originalModel, hit.Model, normalizeMeteringClient(c.GetHeader("User-Agent")), "anthropic", hit.Usage, time.Since(startTime).Milliseconds(), false)
				c.JSON(http.StatusOK, anthropicResp)
				return
			}
			log.Warn().Err(err).Str("request_id", requestIDStr).Msg("Cache hit failed to serialize; falling through to upstream")
		}
	}

	var resolveStart time.Time
	if log.Logger.GetLevel() <= zerolog.DebugLevel {
		resolveStart = time.Now()
	}
	attempts := resolveRequestedModelsForRequest(requestIDStr, canonicalRequest.Model, canonicalRequest)
	logLatencyProbe(requestIDStr, "anthropic_resolve_requested_models", resolveStart)
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
			var candidateStart time.Time
			if log.Logger.GetLevel() <= zerolog.DebugLevel {
				candidateStart = time.Now()
			}
			log.Debug().
				Str("request_id", requestIDStr).
				Str("model", candidate.CanonicalModel).
				Str("provider", providerID).
				Msg("Trying provider for Anthropic request")
			recordAttemptedCandidate(c, providerID, candidate.UpstreamModel)

			log.Debug().
				Str("request_id", requestIDStr).
				Str("provider", providerID).
				Str("api_shape", "anthropic").
				Str("inbound_path", c.FullPath()).
				Str("upstream_api", candidate.UpstreamAPI).
				Str("canonical_model", candidate.CanonicalModel).
				Str("upstream_model", candidate.UpstreamModel).
				Msg("Converted CIF request to upstream model API")

			var err error
			if candidate.Request.Stream {
				err = handleAnthropicStreamingResponse(c, candidate.Adapter, candidate.Request, requestIDStr, originalModel, providerID, startTime)
			} else {
				err = handleAnthropicNonStreamingResponse(c, candidate.Adapter, candidate.Request, requestIDStr, originalModel, providerID, startTime)
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
					Msg("Provider failed for Anthropic request, trying next")
				return err
			}
			affinity.Get().Record(canonicalRequest, candidate.CanonicalModel, providerID)
			logLatencyProbe(requestIDStr, "anthropic_candidate_complete", candidateStart)
			return nil
		},
	)
	if lastErr == nil {
		return
	}
	if resolveFailed {
		writeResolveProvidersError(c, lastErr, "api_error")
		return
	}
	writeProviderFailure(c, "api_error", lastErr)
}

func handleAnthropicNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
	response, err := adapter.Execute(c.Request.Context(), canonicalRequest)
	if err != nil {
		return fmt.Errorf("adapter execute failed: %w", err)
	}

	response = toolarguments.NormalizeResponse(response, canonicalRequest.Tools)

	if response.StopReason == cif.StopReasonToolUse {
		logAnthropicToolLoopResponse(requestID, originalModel, response.Model, providerID, false, extractToolCallLogEntriesFromResponse(response))
	}

	suppressThinking := !strings.Contains(c.GetHeader("anthropic-beta"), "interleaved-thinking")
	anthropicResp, err := serialization.SerializeToAnthropicWithSuppression(response, suppressThinking)
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
	recordUsage(requestID, originalModel, response.Model, providerID, normalizeMeteringClient(c.GetHeader("User-Agent")), "anthropic", response.Usage, time.Since(startTime).Milliseconds(), false, http.StatusOK, "")

	populateResponseCache(c, canonicalRequest, response)

	c.JSON(http.StatusOK, anthropicResp)
	return nil
}

//nolint:gocyclo // streaming response handler with multiple content-type branches
func handleAnthropicStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(c.Request.Context(), canonicalRequest)
	if err != nil {
		if !isClientCanceled(c, err) && shouldFallbackToNonStreaming(err) && allowStreamingFallback(canonicalRequest) {
			log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming request failed before stream start, retrying as non-streaming")
			canonicalRequest.Stream = false
			return handleAnthropicNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
		}
		return err
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ctx := c.Request.Context()
	eventCh = toolarguments.NormalizeStream(ctx, eventCh, canonicalRequest.Tools)

	state := serialization.CreateAnthropicStreamState()
	// Suppress thinking blocks unless the client explicitly opted in to the
	// interleaved-thinking beta.  Non-opted clients (e.g. standard Claude Code)
	// cannot parse thinking blocks and silently stop processing the stream,
	// causing tool_use blocks that follow a thinking block to be ignored.
	if betaHeader := c.GetHeader("anthropic-beta"); !strings.Contains(betaHeader, "interleaved-thinking") {
		state.SuppressThinkingBlocks = true
	}
	flusher, _ := c.Writer.(http.Flusher)
	modelUsed := canonicalRequest.Model
	toolCallTracker := newToolLoopCallTracker()

	acc, cacheState := newResponseCacheStreamAccumulator(c)

	// streamTerminated records whether a well-formed terminal event sequence
	// (message_delta + message_stop, or an error event) was written. If the
	// upstream channel closes or the context is cancelled before that, we
	// synthesise the terminal events so the client does not see a truncated
	// stream ("Connection closed mid-response").
	streamTerminated := false

	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case event, ok := <-eventCh:
			if !ok {
				if !streamTerminated {
					if finalEvents := serialization.FinalizeAnthropicStream(state, cif.StopReasonEndTurn); len(finalEvents) > 0 {
						log.Warn().
							Str("request_id", requestID).
							Str("provider", providerID).
							Str("model_used", modelUsed).
							Int64("latency_ms", time.Since(startTime).Milliseconds()).
							Msg("Upstream stream ended without a terminal event; synthesising message_stop")
						for _, evt := range finalEvents {
							eventType, _ := evt["type"].(string)
							formatted, ferr := serialization.FormatAnthropicSSEData(eventType, evt)
							if ferr != nil {
								break
							}
							fmt.Fprint(w, formatted)
						}
						if flusher != nil {
							flusher.Flush()
						}
						streamTerminated = true
					}
				}
				return false
			}
			toolCallTracker.Observe(event)
			if acc != nil {
				acc.Observe(event)
			}

			sseEvents, err := serialization.ConvertCIFEventToAnthropicSSE(event, state)
			if err != nil {
				log.Error().Err(err).Str("request_id", requestID).Msg("Failed to convert CIF event to Anthropic SSE")
				return false
			}

			for _, sseEvent := range sseEvents {
				eventType, _ := sseEvent["type"].(string)
				formatted, err := serialization.FormatAnthropicSSEData(eventType, sseEvent)
				if err != nil {
					log.Error().Err(err).Str("request_id", requestID).Msg("Failed to format Anthropic SSE event")
					return false
				}
				fmt.Fprint(w, formatted)
			}

			if flusher != nil {
				flusher.Flush()
			}

			if endEvt, isEnd := event.(cif.CIFStreamEnd); isEnd {
				streamTerminated = true
				inputTokens := 0
				outputTokens := 0
				if endEvt.Usage != nil {
					inputTokens = endEvt.Usage.InputTokens
					outputTokens = endEvt.Usage.OutputTokens
				}

				if endEvt.StopReason == cif.StopReasonToolUse {
					logAnthropicToolLoopResponse(requestID, originalModel, modelUsed, providerID, true, toolCallTracker.Entries())
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
				recordUsage(requestID, originalModel, modelUsed, providerID, normalizeMeteringClient(c.GetHeader("User-Agent")), "anthropic", endEvt.Usage, time.Since(startTime).Milliseconds(), true, http.StatusOK, "")

				populateResponseCacheStream(c, canonicalRequest, acc, cacheState)
				return false
			}

			if errEvt, isErr := event.(cif.CIFStreamError); isErr {
				// The error event has already been written to the wire above.
				// Follow it with a well-formed terminal sequence so clients that
				// ignore mid-stream error events still see a complete message
				// rather than a truncated one.
				if finalEvents := serialization.FinalizeAnthropicStream(state, cif.StopReasonEndTurn); len(finalEvents) > 0 {
					for _, evt := range finalEvents {
						eventType, _ := evt["type"].(string)
						formatted, ferr := serialization.FormatAnthropicSSEData(eventType, evt)
						if ferr != nil {
							break
						}
						fmt.Fprint(w, formatted)
					}
					if flusher != nil {
						flusher.Flush()
					}
				}
				streamTerminated = true
				log.Warn().
					Str("request_id", requestID).
					Str("api_shape", "anthropic").
					Str("model_requested", originalModel).
					Str("model_used", modelUsed).
					Str("provider", providerID).
					Str("error_type", errEvt.Error.Type).
					Str("error_message", errEvt.Error.Message).
					Bool("stream", true).
					Int64("latency_ms", time.Since(startTime).Milliseconds()).
					Msg("Anthropic stream ended with error")
				return false
			}

			return true
		}
	})

	return nil
}

func handleCountTokens(c *gin.Context) {
	body, err := readGatewayRequestBody(c.Request.Body)
	if err != nil {
		status, message := gatewayRequestBodyError(err)
		c.JSON(status, gin.H{
			"error": gin.H{
				"message": message,
				"type":    "invalid_request_error",
			},
		})
		return
	}

	canonicalRequest, err := ingestion.ParseAnthropicMessages(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Failed to parse request: %v", err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	totalTokens := 0
	for _, block := range canonicalRequest.System {
		totalTokens += estimateStringTokens(block.Text)
	}
	for _, msg := range canonicalRequest.Messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			for _, block := range m.Content {
				totalTokens += estimateStringTokens(block.Text)
			}
		case cif.CIFUserMessage:
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					totalTokens += estimateStringTokens(p.Text)
				case cif.CIFToolResultPart:
					totalTokens += estimateStringTokens(p.Content)
				}
			}
		case cif.CIFAssistantMessage:
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					totalTokens += estimateStringTokens(p.Text)
				case cif.CIFThinkingPart:
					totalTokens += estimateStringTokens(p.Thinking)
				case cif.CIFToolCallPart:
					totalTokens += estimateStringTokens(p.ToolName)
					if len(p.ToolArguments) > 0 {
						argsBytes, _ := json.Marshal(p.ToolArguments)
						totalTokens += estimateStringTokens(string(argsBytes))
					}
				}
			}
		}
	}

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

func estimateStringTokens(s string) int {
	tokens := len(s) / 4
	if tokens == 0 && len(s) > 0 {
		tokens = 1
	}
	return tokens
}

// replayAnthropicStreamFromCache re-emits a cached CanonicalResponse as an
// Anthropic SSE stream, reusing the production event serializer via synthesized
// CIF events so a streaming Claude Code client sees a normal (instant) stream.
func replayAnthropicStreamFromCache(c *gin.Context, resp *cif.CanonicalResponse, suppressThinking bool) bool {
	state := serialization.CreateAnthropicStreamState()
	state.SuppressThinkingBlocks = suppressThinking
	var payloads []string
	for _, event := range responsecache.SynthesizeStream(resp) {
		sseEvents, err := serialization.ConvertCIFEventToAnthropicSSE(event, state)
		if err != nil {
			return false
		}
		for _, sseEvent := range sseEvents {
			eventType, _ := sseEvent["type"].(string)
			formatted, err := serialization.FormatAnthropicSSEData(eventType, sseEvent)
			if err != nil {
				return false
			}
			payloads = append(payloads, formatted)
		}
	}

	return replayCachedSSE(c, payloads, false)
}
