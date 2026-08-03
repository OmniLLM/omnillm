// HTTP execution for OpenAI-compatible endpoints.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
	"unicode/utf8"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const traceBodyLimit = 1024

const truncationMarker = "...(truncated)"

// cappedBody returns a log-safe copy of b, truncated to traceBodyLimit.
//
// Two properties matter here, and the obvious one-liner gets both wrong.
//
// It must not alias b. The previous implementation used
// append(b[:limit], marker...), which reuses b's backing array whenever
// cap(b) > limit and therefore overwrote live request bytes in place. Callers
// log the body and then send that same buffer upstream, so truncating for
// diagnostics silently corrupted the transmitted payload.
//
// It must not split a rune. Payloads are JSON but carry user content, and a
// severed multi-byte character makes the resulting log field invalid UTF-8.
func cappedBody(b []byte) []byte {
	if len(b) <= traceBodyLimit {
		return b
	}
	cut := utf8SafeCut(b, traceBodyLimit)
	out := make([]byte, 0, cut+len(truncationMarker))
	out = append(out, b[:cut]...)
	out = append(out, truncationMarker...)
	return out
}

// utf8SafeCut returns the largest offset <= limit that lands on a UTF-8
// boundary, so slicing b[:offset] never severs a multi-byte character. The
// limit is a hard ceiling: the result is never larger than limit.
func utf8SafeCut(b []byte, limit int) int {
	if limit >= len(b) {
		return len(b)
	}
	for cut := limit; cut > 0; cut-- {
		if utf8.RuneStart(b[cut]) {
			return cut
		}
	}
	return 0
}

var (
	httpClient = shared.DefaultHTTPClient(
		shared.TimeoutFromEnv("OPENAICOMPAT_HTTP_TIMEOUT", shared.DefaultRequestTimeout))
	// responsesClient is used for the /responses endpoint, which fronts
	// reasoning models that can think for minutes before emitting response
	// headers. Sharing the chat-sized budget made those calls structurally
	// impossible to satisfy rather than merely slow.
	responsesClient = shared.DefaultHTTPClient(
		shared.TimeoutFromEnv("OPENAICOMPAT_RESPONSES_TIMEOUT", shared.DefaultResponsesTimeout))
	streamClient = shared.DefaultStreamClient()
)

// APIError preserves upstream HTTP failures so adapters can decide whether to
// retry on a different upstream API.
type APIError struct {
	StatusCode int
	Body       []byte
}

func (e *APIError) Error() string {
	if e == nil {
		return "openaicompat: upstream request failed"
	}
	return fmt.Sprintf("openaicompat: upstream returned %d: %s", e.StatusCode, string(e.Body))
}

func newPOSTRequest(ctx context.Context, url string, headers map[string]string, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	return req, nil
}

func doPOST(req *http.Request, stream bool) (*http.Response, error) {
	client := httpClient
	if stream {
		client = streamClient
	}
	return doPOSTWith(client, req, stream)
}

// doPOSTResponses executes a /responses POST using the longer-budget client.
func doPOSTResponses(req *http.Request, stream bool) (*http.Response, error) {
	client := responsesClient
	if stream {
		client = streamClient
	}
	return doPOSTWith(client, req, stream)
}

func doPOSTWith(client *http.Client, req *http.Request, stream bool) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		if retryReq := clonePOSTRetryRequest(req, stream, err); retryReq != nil {
			log.Warn().Err(err).Str("url", retryReq.URL.String()).Msg("openaicompat: retrying timed out POST request once")
			resp, err = client.Do(retryReq)
		}
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &APIError{StatusCode: resp.StatusCode, Body: b}
	}
	return resp, nil
}

func clonePOSTRetryRequest(req *http.Request, stream bool, err error) *http.Request {
	if stream || req == nil || req.GetBody == nil || !shouldRetryPOSTTimeout(req, err) {
		return nil
	}
	retryReq := req.Clone(req.Context())
	body, bodyErr := req.GetBody()
	if bodyErr != nil {
		return nil
	}
	retryReq.Body = body
	return retryReq
}

// shouldRetryPOSTTimeout returns true when the failed request should be
// retried once. Currently always false; kept as an extension point.
func shouldRetryPOSTTimeout(_ *http.Request, _ error) bool {
	return false
}

func startSSEStream(ctx context.Context, body io.ReadCloser, parser func(context.Context, io.ReadCloser, chan cif.CIFStreamEvent)) <-chan cif.CIFStreamEvent {
	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parser(ctx, body, eventCh)
	return eventCh
}

// Execute performs a non-streaming POST to url and returns a CIF response.
func Execute(ctx context.Context, url string, headers map[string]string, cr *ChatRequest) (*cif.CanonicalResponse, error) {
	cr.Stream = false
	body, err := Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal request: %w", err)
	}

	if log.Logger.GetLevel() <= zerolog.TraceLevel {
		log.Trace().Str("url", url).RawJSON("payload", cappedBody(body)).Msg("outbound openaicompat request")
	}

	req, err := newPOSTRequest(ctx, url, headers, body, false)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: create request: %w", err)
	}

	resp, err := doPOST(req, false)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: request failed: %w", err)
	}
	defer resp.Body.Close()

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("openaicompat: decode response: %w", err)
	}
	return ParseChatResponse(&chatResp), nil
}

// Stream performs a streaming POST to url and returns a CIF event channel.
func Stream(ctx context.Context, url string, headers map[string]string, cr *ChatRequest) (<-chan cif.CIFStreamEvent, error) {
	cr.Stream = true
	body, err := Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal request: %w", err)
	}

	if log.Logger.GetLevel() <= zerolog.TraceLevel {
		log.Trace().Str("url", url).RawJSON("payload", cappedBody(body)).Msg("outbound openaicompat stream request")
	}

	req, err := newPOSTRequest(ctx, url, headers, body, true)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: create stream request: %w", err)
	}

	resp, err := doPOST(req, true)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: stream request failed: %w", err)
	}

	return startSSEStream(ctx, resp.Body, ParseSSE), nil
}

// CollectStream is a convenience wrapper: runs Stream and assembles CIF response.
func CollectStream(ctx context.Context, url string, headers map[string]string, cr *ChatRequest) (*cif.CanonicalResponse, error) {
	ch, err := Stream(ctx, url, headers, cr)
	if err != nil {
		return nil, err
	}
	return shared.CollectStream(ch)
}
