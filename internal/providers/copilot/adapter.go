package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func debugEnabled() bool {
	return log.Logger.GetLevel() <= zerolog.DebugLevel
}

func logCopilotElapsed(providerID, endpoint, model string, started time.Time, message string) {
	log.Debug().
		Str("provider", providerID).
		Str("endpoint", endpoint).
		Str("model", model).
		Int64("elapsed_ms", time.Since(started).Milliseconds()).
		Msg(message)
}

func (p *GitHubCopilotProvider) GetAdapter() types.ProviderAdapter {
	return &CopilotAdapter{provider: p}
}

// CIF Adapter implementation
func (a *CopilotAdapter) GetProvider() types.Provider {
	return a.provider
}

func (a *CopilotAdapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	model := ""
	if request != nil {
		model = a.RemapModel(request.Model)
	}
	switch a.selectShape(model, request) {
	case shapeResponses:
		return a.executeResponses(ctx, request)
	default:
		return a.executeOpenAI(ctx, request)
	}
}

func (a *CopilotAdapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if a.shouldBufferAnthropicStreaming(request) {
		var started time.Time
		if debugEnabled() {
			started = time.Now()
		}
		response, err := a.Execute(ctx, request)
		if err != nil {
			return nil, err
		}
		if debugEnabled() {
			logCopilotElapsed(a.provider.GetInstanceID(), "buffered-anthropic-stream", request.Model, started, "Copilot buffered Anthropic streaming")
		}
		return shared.StreamResponse(response), nil
	}

	model := ""
	if request != nil {
		model = a.RemapModel(request.Model)
	}
	switch a.selectShape(model, request) {
	case shapeResponses:
		return a.executeResponsesStream(ctx, request)
	default:
		return a.executeOpenAIStream(ctx, request)
	}
}

func (a *CopilotAdapter) shouldBufferAnthropicStreaming(request *cif.CanonicalRequest) bool {
	if request == nil || request.Extensions == nil || request.Extensions.InboundAPIShape == nil {
		return false
	}

	if !strings.EqualFold(strings.TrimSpace(*request.Extensions.InboundAPIShape), "anthropic") {
		return false
	}

	// Copilot's Claude chat-completions stream carries valid incremental tool_calls,
	// while the buffered non-streaming path can collapse into stop_reason=tool_use
	// without an actual tool block. Preserve native streaming for Claude models.
	model := a.RemapModel(request.Model)
	return !strings.Contains(strings.ToLower(model), "claude")
}

func (a *CopilotAdapter) forceChatCompletions(request *cif.CanonicalRequest) bool {
	return request != nil &&
		request.Extensions != nil &&
		request.Extensions.ForceChatCompletions != nil &&
		*request.Extensions.ForceChatCompletions
}

// isUnsupportedChatCompletionsModel detects Copilot's
// `unsupported_api_for_model` 400 so we can fall back to /responses.
func (a *CopilotAdapter) isUnsupportedChatCompletionsModel(apiErr *copilotAPIError) bool {
	if apiErr == nil || apiErr.statusCode != http.StatusBadRequest {
		return false
	}
	var payload copilotErrorEnvelope
	if err := json.Unmarshal(apiErr.body, &payload); err == nil {
		if payload.Error.Code == "unsupported_api_for_model" {
			return true
		}
		if strings.Contains(strings.ToLower(payload.Error.Message), "/chat/completions") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(string(apiErr.body)), "unsupported_api_for_model")
}

func (a *CopilotAdapter) RemapModel(canonicalModel string) string {
	return modelrouting.NormalizeModelName(canonicalModel)
}

func (a *CopilotAdapter) chatCompletionsClient(model string, stream bool) (*http.Client, time.Duration) {
	if strings.Contains(strings.ToLower(a.RemapModel(model)), "claude") {
		if stream {
			return copilotClaudeClient, copilotClaudeHeaderTimeout
		}
		return copilotClaudeBoundedClient, copilotClaudeHeaderTimeout
	}
	if stream {
		return copilotChatStreamClient, copilotHTTPClient.Timeout
	}
	return copilotHTTPClient, copilotHTTPClient.Timeout
}

func (a *CopilotAdapter) logUpstreamTimeout(ctx context.Context, request *cif.CanonicalRequest, endpoint string, budget, elapsed time.Duration, err error) {
	if err == nil || !isTimeoutError(err) {
		return
	}

	event := zerolog.Ctx(ctx).Warn().
		Err(err).
		Str("provider", a.provider.GetInstanceID()).
		Str("endpoint", endpoint).
		Int64("timeout_budget_ms", budget.Milliseconds()).
		Int64("elapsed_ms", elapsed.Milliseconds())
	if request != nil {
		event = event.Str("model", request.Model)
	}
	event.Msg("Copilot upstream timed out awaiting response headers")
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return (errors.As(err, &netErr) && netErr.Timeout()) ||
		strings.Contains(strings.ToLower(err.Error()), "client.timeout exceeded")
}

func (a *CopilotAdapter) executeOpenAI(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return a.executeOpenAIWithRetry(ctx, request, true, true)
}

// logTransportRetry emits the single structured warning that accompanies a
// transient-transport retry. It carries no credentials or request content.
func (a *CopilotAdapter) logTransportRetry(request *cif.CanonicalRequest, endpoint, reason string, statusCode int) {
	event := log.Warn().
		Str("provider", a.provider.GetInstanceID()).
		Str("endpoint", endpoint).
		Str("retry_reason", reason).
		Int("attempt", 2)
	if request != nil {
		event = event.Str("model", request.Model)
	}
	if statusCode > 0 {
		event = event.Int("upstream_status", statusCode)
	}
	event.Msg("Copilot upstream transient transport failure, retrying once")
}

func (a *CopilotAdapter) executeOpenAIWithRetry(ctx context.Context, request *cif.CanonicalRequest, allowAuthRetry, allowTransportRetry bool) (*cif.CanonicalResponse, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	openaiPayload := a.convertCIFToOpenAI(request, toolNameMapper)
	openaiPayload["stream"] = false

	reqBody, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}

	started := time.Now()
	client, budget := a.chatCompletionsClient(request.Model, false)
	resp, err := client.Do(req)
	elapsed := time.Since(started)
	if debugEnabled() {
		logCopilotElapsed(a.provider.GetInstanceID(), "chat.completions", request.Model, started, "Copilot upstream request completed")
	}
	if err != nil {
		a.logUpstreamTimeout(ctx, request, "chat.completions", budget, elapsed, err)
		if allowTransportRetry {
			if transient, reason := isTransientTransportFailure(err, 0); transient {
				a.logTransportRetry(request, "chat.completions", reason, 0)
				if waitBeforeTransportRetry(ctx) {
					return a.executeOpenAIWithRetry(ctx, request, allowAuthRetry, false)
				}
			}
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: body}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("chat.completions") {
			return a.executeOpenAIWithRetry(ctx, request, false, allowTransportRetry)
		}
		if allowTransportRetry {
			if transient, reason := isTransientTransportFailure(nil, resp.StatusCode); transient {
				a.logTransportRetry(request, "chat.completions", reason, resp.StatusCode)
				if waitBeforeTransportRetry(ctx) {
					return a.executeOpenAIWithRetry(ctx, request, allowAuthRetry, false)
				}
			}
		}
		if !a.forceChatCompletions(request) && a.isUnsupportedChatCompletionsModel(apiErr) {
			log.Info().
				Str("model", request.Model).
				Str("provider", a.provider.GetInstanceID()).
				Msg("Copilot model requires responses API, retrying request")
			return a.executeResponses(ctx, request)
		}
		return nil, apiErr
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.convertOpenAIToCIF(openaiResp, toolNameMapper), nil
}

func (a *CopilotAdapter) executeOpenAIStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.executeOpenAIStreamWithRetry(ctx, request, true, true)
}

func (a *CopilotAdapter) executeOpenAIStreamWithRetry(ctx context.Context, request *cif.CanonicalRequest, allowAuthRetry, allowTransportRetry bool) (<-chan cif.CIFStreamEvent, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	openaiPayload := a.convertCIFToOpenAI(request, toolNameMapper)
	openaiPayload["stream"] = true

	reqBody, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	started := time.Now()
	client, budget := a.chatCompletionsClient(request.Model, true)
	resp, err := client.Do(req)
	elapsed := time.Since(started)
	if debugEnabled() {
		logCopilotElapsed(a.provider.GetInstanceID(), "chat.completions-stream", request.Model, started, "Copilot upstream request completed")
	}
	if err != nil {
		a.logUpstreamTimeout(ctx, request, "chat.completions-stream", budget, elapsed, err)
		// Safe to retry: no response body exists yet, so parseOpenAISSE has
		// not started and no stream event can have reached the caller.
		if allowTransportRetry {
			if transient, reason := isTransientTransportFailure(err, 0); transient {
				a.logTransportRetry(request, "chat.completions-stream", reason, 0)
				if waitBeforeTransportRetry(ctx) {
					return a.executeOpenAIStreamWithRetry(ctx, request, allowAuthRetry, false)
				}
			}
		}
		return nil, fmt.Errorf("streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: body}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("chat.completions-stream") {
			return a.executeOpenAIStreamWithRetry(ctx, request, false, allowTransportRetry)
		}
		if allowTransportRetry {
			if transient, reason := isTransientTransportFailure(nil, resp.StatusCode); transient {
				a.logTransportRetry(request, "chat.completions-stream", reason, resp.StatusCode)
				if waitBeforeTransportRetry(ctx) {
					return a.executeOpenAIStreamWithRetry(ctx, request, allowAuthRetry, false)
				}
			}
		}
		if !a.forceChatCompletions(request) && a.isUnsupportedChatCompletionsModel(apiErr) {
			log.Info().
				Str("model", request.Model).
				Str("provider", a.provider.GetInstanceID()).
				Msg("Copilot model requires responses API for streaming, retrying request")
			return a.executeResponsesStream(ctx, request)
		}
		return nil, apiErr
	}

	// Past this point the SSE goroutine owns the body. Any later failure is
	// reported through the event channel and is deliberately unreachable from
	// the retry path above, which is what guarantees we never replay output
	// the caller has already seen.
	eventCh := make(chan cif.CIFStreamEvent, 64)
	go a.parseOpenAISSE(ctx, resp.Body, eventCh, toolNameMapper)
	return eventCh, nil
}

func (a *CopilotAdapter) shouldRetryAfterAuthError(request *cif.CanonicalRequest, apiErr *copilotAPIError) bool {
	if request != nil &&
		request.Extensions != nil &&
		request.Extensions.DisableAuthRetry != nil &&
		*request.Extensions.DisableAuthRetry {
		return false
	}

	return apiErr != nil && apiErr.IsAuthenticationError() && a.provider.githubToken != ""
}

func (a *CopilotAdapter) requestHeaders(request *cif.CanonicalRequest) map[string]string {
	headers := a.provider.GetHeaders(a.requestUsesVision(request))
	headers["X-Initiator"] = a.requestInitiator(request)
	return headers
}

func (a *CopilotAdapter) requestUsesVision(request *cif.CanonicalRequest) bool {
	if request == nil {
		return false
	}

	for _, message := range request.Messages {
		for _, part := range messageContentParts(message) {
			if _, ok := part.(cif.CIFImagePart); ok {
				return true
			}
		}
	}

	return false
}

func (a *CopilotAdapter) requestInitiator(request *cif.CanonicalRequest) string {
	if request == nil {
		return "user"
	}

	for _, message := range request.Messages {
		switch msg := message.(type) {
		case cif.CIFAssistantMessage:
			if len(msg.Content) > 0 {
				return "agent"
			}
		case cif.CIFUserMessage:
			for _, part := range msg.Content {
				if _, ok := part.(cif.CIFToolResultPart); ok {
					return "agent"
				}
			}
		}
	}

	return "user"
}

func messageContentParts(message cif.CIFMessage) []cif.CIFContentPart {
	switch msg := message.(type) {
	case cif.CIFUserMessage:
		return msg.Content
	case cif.CIFAssistantMessage:
		return msg.Content
	default:
		return nil
	}
}

func (a *CopilotAdapter) refreshTokenForRetry(endpoint string) bool {
	if err := a.provider.RefreshToken(); err != nil {
		log.Warn().
			Err(err).
			Str("provider", a.provider.GetInstanceID()).
			Str("endpoint", endpoint).
			Msg("Failed to refresh Copilot token after upstream auth error")
		return false
	}

	log.Info().
		Str("provider", a.provider.GetInstanceID()).
		Str("endpoint", endpoint).
		Msg("Refreshed Copilot token after upstream auth error, retrying request")
	return true
}
