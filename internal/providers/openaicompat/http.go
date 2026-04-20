// HTTP execution for OpenAI-compatible endpoints.
package openaicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"

	"github.com/rs/zerolog/log"
)

var (
	httpClient   = &http.Client{Timeout: 120 * time.Second}
	streamClient = &http.Client{}
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

func newPOSTRequest(url string, headers map[string]string, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &APIError{StatusCode: resp.StatusCode, Body: b}
	}
	return resp, nil
}

func startSSEStream(body io.ReadCloser, parser func(io.ReadCloser, chan cif.CIFStreamEvent)) <-chan cif.CIFStreamEvent {
	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parser(body, eventCh)
	return eventCh
}

// Execute performs a non-streaming POST to url and returns a CIF response.
func Execute(url string, headers map[string]string, cr *ChatRequest) (*cif.CanonicalResponse, error) {
	cr.Stream = false
	body, err := Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound openaicompat request")

	req, err := newPOSTRequest(url, headers, body, false)
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
func Stream(url string, headers map[string]string, cr *ChatRequest) (<-chan cif.CIFStreamEvent, error) {
	cr.Stream = true
	body, err := Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound openaicompat stream request")

	req, err := newPOSTRequest(url, headers, body, true)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: create stream request: %w", err)
	}

	resp, err := doPOST(req, true)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: stream request failed: %w", err)
	}

	return startSSEStream(resp.Body, ParseSSE), nil
}

// CollectStream is a convenience wrapper: runs Stream and assembles CIF response.
func CollectStream(url string, headers map[string]string, cr *ChatRequest) (*cif.CanonicalResponse, error) {
	ch, err := Stream(url, headers, cr)
	if err != nil {
		return nil, err
	}
	return shared.CollectStream(ch)
}
