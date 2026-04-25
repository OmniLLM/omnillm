package generic

import (
	"bytes"
	"context"
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
	genericHTTPClient = &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	genericStreamClient = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
)

// executeOpenAIWithPayload performs a non-streaming OpenAI-compatible HTTP request.
func executeOpenAIWithPayload(ctx context.Context, url string, headers map[string]string, payload map[string]interface{}, originalModel string) (*cif.CanonicalResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound proxy request payload")

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := genericHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return shared.OpenAIRespToCIF(openaiResp), nil
}

// streamOpenAIWithPayload performs a streaming OpenAI-compatible HTTP request.
func streamOpenAIWithPayload(ctx context.Context, url string, headers map[string]string, payload map[string]interface{}) (<-chan cif.CIFStreamEvent, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound proxy request payload")

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := genericStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("streaming API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go shared.ParseOpenAISSE(resp.Body, eventCh)
	return eventCh, nil
}

// firstString returns the first non-empty string value for the given keys in a map.
func firstString(values map[string]interface{}, keys ...string) (string, bool) {
	return shared.FirstString(values, keys...)
}

func stringValueOrEmpty(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
