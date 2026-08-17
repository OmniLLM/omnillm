package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/ingestion"
	"omnillm/internal/lib/responsecache"
)

func enableResponseCacheForTest(t *testing.T) {
	t.Helper()
	store := database.NewConfigStore()
	originalEnabled, enabledErr := store.Get("response_cache.enabled")
	originalTTL, ttlErr := store.Get("response_cache.ttl_seconds")
	if err := store.Set("response_cache.enabled", "true"); err != nil {
		t.Fatalf("enable response cache: %v", err)
	}
	if err := store.Set("response_cache.ttl_seconds", "3600"); err != nil {
		t.Fatalf("set response cache TTL: %v", err)
	}
	t.Cleanup(func() {
		if enabledErr == nil {
			_ = store.Set("response_cache.enabled", originalEnabled)
		} else {
			_ = store.Set("response_cache.enabled", "false")
		}
		if ttlErr == nil {
			_ = store.Set("response_cache.ttl_seconds", originalTTL)
		}
	})
}

type memoryResponseCacheStore struct {
	mu      sync.Mutex
	records map[string]*responsecache.Record
	hits    int64
}

func newMemoryResponseCacheStore() *memoryResponseCacheStore {
	return &memoryResponseCacheStore{records: make(map[string]*responsecache.Record)}
}

func (s *memoryResponseCacheStore) Get(_ context.Context, key string) (*responsecache.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[key]
	if record == nil {
		return nil, nil
	}
	s.hits++
	copy := *record
	copy.HitCount = s.hits
	return &copy, nil
}
func (s *memoryResponseCacheStore) Save(_ context.Context, key, modelID, data string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[key] = &responsecache.Record{Key: key, ModelID: modelID, ResponseData: data}
	return nil
}
func (s *memoryResponseCacheStore) Stats(context.Context) (responsecache.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return responsecache.Stats{Entries: int64(len(s.records)), TotalHits: s.hits}, nil
}
func (s *memoryResponseCacheStore) Clear(context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := int64(len(s.records))
	clear(s.records)
	return count, nil
}
func (s *memoryResponseCacheStore) Available() bool { return true }
func (s *memoryResponseCacheStore) Close() error    { return nil }

func newResponseCacheTestServer(t *testing.T) (*httptest.Server, *memoryResponseCacheStore) {
	t.Helper()
	enableResponseCacheForTest(t)
	store := newMemoryResponseCacheStore()
	restore := responsecache.ConfigureStore(store)
	t.Cleanup(restore)
	server := newTestServer(t)
	t.Cleanup(server.Close)
	return server, store
}

func TestResponseCachePopulatesHitsBypassAndOff(t *testing.T) {
	srv, _ := newResponseCacheTestServer(t)

	calls := 0
	registerStubProvider(t, "cache-hit-model", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		calls++
		return &cif.CanonicalResponse{
			ID:         "cache-hit",
			Model:      request.Model,
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "cached-upstream"}},
			StopReason: cif.StopReasonEndTurn,
		}, nil
	}, nil)

	body := `{"model":"cache-hit-model","temperature":0,"messages":[{"role":"user","content":"same"}]}`
	request := func(control string) *http.Response {
		headers := map[string]string{}
		if control != "" {
			headers[responsecache.BypassHeader] = control
		}
		return postJSON(t, srv.URL+"/v1/chat/completions", body, headers)
	}

	for _, step := range []struct {
		control string
		header  string
		calls   int
	}{
		{header: "miss", calls: 1},
		{header: "hit", calls: 1},
		{control: "bypass", header: "miss", calls: 2},
		{control: "off", header: "", calls: 3},
	} {
		response := request(step.control)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", response.StatusCode)
		}
		if got := response.Header.Get("X-OmniLLM-Cache"); got != step.header {
			t.Fatalf("control %q header = %q, want %q", step.control, got, step.header)
		}
		if calls != step.calls {
			t.Fatalf("control %q upstream calls = %d, want %d", step.control, calls, step.calls)
		}
	}
}

func TestResponseCacheCrossDialectAndStreamingReplay(t *testing.T) {
	srv, _ := newResponseCacheTestServer(t)

	calls := 0
	registerStubProvider(t, "cache-cross-mode-model", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		calls++
		return &cif.CanonicalResponse{
			ID:         "cache-cross-mode",
			Model:      request.Model,
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "cross-mode-answer"}},
			StopReason: cif.StopReasonEndTurn,
		}, nil
	}, nil)

	chatBody := `{"model":"cache-cross-mode-model","temperature":0,"max_tokens":64,"messages":[{"role":"user","content":"same prompt"}]}`
	first := postJSON(t, srv.URL+"/v1/chat/completions", chatBody, nil)
	first.Body.Close()
	if first.StatusCode != http.StatusOK || first.Header.Get("X-OmniLLM-Cache") != "miss" {
		t.Fatalf("population status=%d cache=%q", first.StatusCode, first.Header.Get("X-OmniLLM-Cache"))
	}

	messagesBody := `{"model":"cache-cross-mode-model","temperature":0,"max_tokens":64,"messages":[{"role":"user","content":"same prompt"}]}`
	anthropic := postJSON(t, srv.URL+"/v1/messages", messagesBody, map[string]string{"anthropic-version": "2023-06-01"})
	anthropicBody := readBody(t, anthropic)
	if anthropic.StatusCode != http.StatusOK || anthropic.Header.Get("X-OmniLLM-Cache") != "hit" {
		t.Fatalf("Anthropic replay status=%d cache=%q body=%s", anthropic.StatusCode, anthropic.Header.Get("X-OmniLLM-Cache"), anthropicBody)
	}

	streamBody := `{"model":"cache-cross-mode-model","temperature":0,"max_tokens":64,"stream":true,"messages":[{"role":"user","content":"same prompt"}]}`
	stream := postJSON(t, srv.URL+"/v1/chat/completions", streamBody, nil)
	streamResponse := readBody(t, stream)
	if stream.StatusCode != http.StatusOK || stream.Header.Get("X-OmniLLM-Cache") != "hit" {
		t.Fatalf("stream replay status=%d cache=%q body=%s", stream.StatusCode, stream.Header.Get("X-OmniLLM-Cache"), streamResponse)
	}
	if !strings.Contains(streamResponse, "cross-mode-answer") || !strings.Contains(streamResponse, "[DONE]") {
		t.Fatalf("stream replay missing content or terminal event: %s", streamResponse)
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
}

func TestResponseCacheFailureFallsThroughAllGenerationRoutes(t *testing.T) {
	enableResponseCacheForTest(t)
	restore := responsecache.ConfigureStore(nil)
	defer restore()

	calls := 0
	registerStubProvider(t, "cache-fail-open-model", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		calls++
		return &cif.CanonicalResponse{
			ID:         "cache-fail-open",
			Model:      request.Model,
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "upstream-ok"}},
			StopReason: cif.StopReasonEndTurn,
		}, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	rows := []struct {
		name     string
		endpoint string
		body     string
		headers  map[string]string
	}{
		{
			name:     "chat",
			endpoint: "/v1/chat/completions",
			body:     `{"model":"cache-fail-open-model","temperature":0,"messages":[{"role":"user","content":"hello"}]}`,
		},
		{
			name:     "messages",
			endpoint: "/v1/messages",
			body:     `{"model":"cache-fail-open-model","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`,
			headers:  map[string]string{"anthropic-version": "2023-06-01"},
		},
		{
			name:     "responses",
			endpoint: "/v1/responses",
			body:     `{"model":"cache-fail-open-model","input":"hello"}`,
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			response := postJSON(t, srv.URL+row.endpoint, row.body, row.headers)
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", response.StatusCode)
			}
			if got := response.Header.Get("X-OmniLLM-Cache"); got != "miss" {
				t.Fatalf("cache header = %q, want miss", got)
			}
		})
	}
	if calls != len(rows) {
		t.Fatalf("upstream calls = %d, want %d", calls, len(rows))
	}
}

func TestResponseCacheNonStreamingAcrossGenerationRoutes(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		body     func(string) string
		headers  map[string]string
	}{
		{
			name:     "chat omitted sampling",
			endpoint: "/v1/chat/completions",
			body: func(model string) string {
				return fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"same"}]}`, model)
			},
		},
		{
			name:     "messages nonzero sampling",
			endpoint: "/v1/messages",
			body: func(model string) string {
				return fmt.Sprintf(`{"model":%q,"temperature":0.7,"max_tokens":64,"messages":[{"role":"user","content":"same"}]}`, model)
			},
			headers: map[string]string{"anthropic-version": "2023-06-01"},
		},
		{
			name:     "responses omitted sampling",
			endpoint: "/v1/responses",
			body: func(model string) string {
				return fmt.Sprintf(`{"model":%q,"input":"same"}`, model)
			},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv, _ := newResponseCacheTestServer(t)

			model := fmt.Sprintf("cache-route-%d", index)
			calls := 0
			registerStubProvider(t, model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
				calls++
				return &cif.CanonicalResponse{
					ID:         "route-hit",
					Model:      request.Model,
					Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "cached-route-answer"}},
					StopReason: cif.StopReasonEndTurn,
					Usage:      &cif.CIFUsage{InputTokens: 9, OutputTokens: 3},
				}, nil
			}, nil)

			for requestIndex, wantHeader := range []string{"miss", "hit"} {
				response := postJSON(t, srv.URL+test.endpoint, test.body(model), test.headers)
				body := readBody(t, response)
				if response.StatusCode != http.StatusOK {
					t.Fatalf("request %d status=%d body=%s", requestIndex, response.StatusCode, body)
				}
				if got := response.Header.Get(responsecache.BypassHeader); got != wantHeader {
					t.Fatalf("request %d cache header=%q, want %q", requestIndex, got, wantHeader)
				}
				if !strings.Contains(body, "cached-route-answer") {
					t.Fatalf("request %d lost cached output: %s", requestIndex, body)
				}
			}
			if calls != 1 {
				t.Fatalf("upstream calls=%d, want 1", calls)
			}
		})
	}
}

func TestResponsesCacheControls(t *testing.T) {
	srv, _ := newResponseCacheTestServer(t)

	calls := 0
	registerStubProvider(t, "cache-responses-controls", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		calls++
		return &cif.CanonicalResponse{
			ID:         fmt.Sprintf("controls-%d", calls),
			Model:      request.Model,
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: fmt.Sprintf("answer-%d", calls)}},
			StopReason: cif.StopReasonEndTurn,
		}, nil
	}, nil)

	body := `{"model":"cache-responses-controls","input":"same"}`

	steps := []struct {
		control string
		header  string
		calls   int
		answer  string
	}{
		{header: "miss", calls: 1, answer: "answer-1"},
		{header: "hit", calls: 1, answer: "answer-1"},
		{control: "refresh", header: "miss", calls: 2, answer: "answer-2"},
		{header: "hit", calls: 2, answer: "answer-2"},
		{control: "disable", header: "", calls: 3, answer: "answer-3"},
		{header: "hit", calls: 3, answer: "answer-2"},
	}
	for _, step := range steps {
		headers := map[string]string{}
		if step.control != "" {
			headers[responsecache.BypassHeader] = step.control
		}
		response := postJSON(t, srv.URL+"/v1/responses", body, headers)
		responseBody := readBody(t, response)
		if response.StatusCode != http.StatusOK || response.Header.Get(responsecache.BypassHeader) != step.header {
			t.Fatalf("control=%q status=%d cache=%q body=%s", step.control, response.StatusCode, response.Header.Get(responsecache.BypassHeader), responseBody)
		}
		if calls != step.calls || !strings.Contains(responseBody, step.answer) {
			t.Fatalf("control=%q calls=%d body=%s", step.control, calls, responseBody)
		}
	}
}

func TestResponseCacheTrueStreamPopulationAllDialects(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		body       func(string) string
		headers    map[string]string
		terminal   string
		deltaValue string
	}{
		{
			name:     "chat",
			endpoint: "/v1/chat/completions",
			body: func(model string) string {
				return fmt.Sprintf(`{"model":%q,"stream":true,"messages":[{"role":"user","content":"same"}]}`, model)
			},
			terminal:   "data: [DONE]",
			deltaValue: `"content":"streamed"`,
		},
		{
			name:     "messages",
			endpoint: "/v1/messages",
			body: func(model string) string {
				return fmt.Sprintf(`{"model":%q,"stream":true,"max_tokens":64,"messages":[{"role":"user","content":"same"}]}`, model)
			},
			headers:    map[string]string{"anthropic-version": "2023-06-01"},
			terminal:   "event: message_stop",
			deltaValue: `"text":"streamed"`,
		},
		{
			name:     "responses",
			endpoint: "/v1/responses",
			body: func(model string) string {
				return fmt.Sprintf(`{"model":%q,"stream":true,"input":"same"}`, model)
			},
			terminal:   "event: response.completed",
			deltaValue: `"delta":"streamed"`,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv, _ := newResponseCacheTestServer(t)
			model := fmt.Sprintf("cache-true-stream-%d", index)
			calls := 0
			registerStubProvider(t, model, nil, func(_ context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
				calls++
				ch := make(chan cif.CIFStreamEvent, 3)
				ch <- cif.CIFStreamStart{Type: "stream_start", ID: "stream-all", Model: request.Model}
				ch <- cif.CIFContentDelta{Type: "content_delta", Index: 0, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "streamed"}}
				ch <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn, Usage: &cif.CIFUsage{InputTokens: 6, OutputTokens: 1}}
				close(ch)
				return ch, nil
			})

			for requestIndex, wantHeader := range []string{"miss", "hit"} {
				response := postJSON(t, srv.URL+test.endpoint, test.body(model), test.headers)
				body := readBody(t, response)
				if response.StatusCode != http.StatusOK || response.Header.Get(responsecache.BypassHeader) != wantHeader {
					t.Fatalf("request %d status=%d cache=%q body=%s", requestIndex, response.StatusCode, response.Header.Get(responsecache.BypassHeader), body)
				}
				if strings.Count(body, test.terminal) != 1 || strings.Count(body, test.deltaValue) != 1 {
					t.Fatalf("request %d duplicate/missing stream output: %s", requestIndex, body)
				}
			}
			if calls != 1 {
				t.Fatalf("upstream stream calls=%d, want 1", calls)
			}
		})
	}
}

func TestResponsesCacheStreamingPopulationAndCrossModeReplay(t *testing.T) {
	t.Run("stream populates stream and nonstream", func(t *testing.T) {
		srv, _ := newResponseCacheTestServer(t)

		streamCalls := 0
		registerStubProvider(t, "cache-responses-stream", nil, func(_ context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
			streamCalls++
			ch := make(chan cif.CIFStreamEvent, 3)
			ch <- cif.CIFStreamStart{Type: "stream_start", ID: "responses-stream", Model: request.Model}
			ch <- cif.CIFContentDelta{Type: "content_delta", Index: 0, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "stream-cached"}}
			ch <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn, Usage: &cif.CIFUsage{InputTokens: 7, OutputTokens: 2}}
			close(ch)
			return ch, nil
		})

		streamBody := `{"model":"cache-responses-stream","stream":true,"input":"same"}`
		for index, wantHeader := range []string{"miss", "hit"} {
			response := postJSON(t, srv.URL+"/v1/responses", streamBody, nil)
			body := readBody(t, response)
			if response.StatusCode != http.StatusOK || response.Header.Get(responsecache.BypassHeader) != wantHeader {
				t.Fatalf("stream %d status=%d cache=%q body=%s", index, response.StatusCode, response.Header.Get(responsecache.BypassHeader), body)
			}
			if strings.Count(body, "event: response.completed") != 1 || strings.Count(body, "stream-cached") == 0 {
				t.Fatalf("stream %d terminal/output invalid: %s", index, body)
			}
		}
		nonstream := postJSON(t, srv.URL+"/v1/responses", `{"model":"cache-responses-stream","input":"same"}`, nil)
		nonstreamBody := readBody(t, nonstream)
		if nonstream.StatusCode != http.StatusOK || nonstream.Header.Get(responsecache.BypassHeader) != "hit" || !strings.Contains(nonstreamBody, "stream-cached") {
			t.Fatalf("nonstream replay status=%d cache=%q body=%s", nonstream.StatusCode, nonstream.Header.Get(responsecache.BypassHeader), nonstreamBody)
		}
		if streamCalls != 1 {
			t.Fatalf("stream calls=%d, want 1", streamCalls)
		}
	})

	t.Run("nonstream populates stream", func(t *testing.T) {
		srv, _ := newResponseCacheTestServer(t)
		calls := 0
		registerStubProvider(t, "cache-responses-nonstream-to-stream", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			calls++
			return &cif.CanonicalResponse{ID: "nonstream-source", Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "cross-mode"}}, StopReason: cif.StopReasonEndTurn}, nil
		}, nil)

		population := postJSON(t, srv.URL+"/v1/responses", `{"model":"cache-responses-nonstream-to-stream","input":"same"}`, nil)
		population.Body.Close()
		replay := postJSON(t, srv.URL+"/v1/responses", `{"model":"cache-responses-nonstream-to-stream","stream":true,"input":"same"}`, nil)
		body := readBody(t, replay)
		if replay.StatusCode != http.StatusOK || replay.Header.Get(responsecache.BypassHeader) != "hit" || strings.Count(body, "event: response.completed") != 1 || strings.Count(body, `"delta":"cross-mode"`) != 1 {
			t.Fatalf("stream replay status=%d cache=%q body=%s", replay.StatusCode, replay.Header.Get(responsecache.BypassHeader), body)
		}
		if calls != 1 {
			t.Fatalf("upstream calls=%d, want 1", calls)
		}
	})
}

func TestResponsesCacheDoesNotPopulateIncompleteOrErroredStream(t *testing.T) {
	for _, test := range []struct {
		name   string
		events func(*cif.CanonicalRequest) []cif.CIFStreamEvent
	}{
		{
			name: "incomplete closure",
			events: func(request *cif.CanonicalRequest) []cif.CIFStreamEvent {
				return []cif.CIFStreamEvent{
					cif.CIFStreamStart{Type: "stream_start", ID: "incomplete", Model: request.Model},
					cif.CIFContentDelta{Type: "content_delta", Index: 0, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "partial"}},
				}
			},
		},
		{
			name: "error event",
			events: func(request *cif.CanonicalRequest) []cif.CIFStreamEvent {
				return []cif.CIFStreamEvent{
					cif.CIFStreamStart{Type: "stream_start", ID: "errored", Model: request.Model},
					cif.CIFStreamError{Type: "stream_error", Error: cif.ErrorInfo{Type: "upstream_error", Message: "failed"}},
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv, store := newResponseCacheTestServer(t)
			model := "cache-responses-" + strings.ReplaceAll(test.name, " ", "-")
			calls := 0
			registerStubProvider(t, model, nil, func(_ context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
				calls++
				events := test.events(request)
				ch := make(chan cif.CIFStreamEvent, len(events))
				for _, event := range events {
					ch <- event
				}
				close(ch)
				return ch, nil
			})
			body := fmt.Sprintf(`{"model":%q,"stream":true,"input":"same"}`, model)
			for range 2 {
				response := postJSON(t, srv.URL+"/v1/responses", body, nil)
				_ = readBody(t, response)
				if response.Header.Get(responsecache.BypassHeader) != "miss" {
					t.Fatalf("cache header=%q, want miss", response.Header.Get(responsecache.BypassHeader))
				}
			}
			if calls != 2 {
				t.Fatalf("upstream calls=%d, want 2", calls)
			}
			stats, _ := store.Stats(context.Background())
			if stats.Entries != 0 {
				t.Fatalf("incomplete/error stream populated %d entries", stats.Entries)
			}
		})
	}
}

func TestResponsesCacheDoesNotPopulateCanceledStream(t *testing.T) {
	srv, store := newResponseCacheTestServer(t)
	providerCanceled := make(chan struct{})
	registerStubProvider(t, "cache-responses-canceled", nil, func(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
		ch := make(chan cif.CIFStreamEvent)
		go func() {
			defer close(ch)
			select {
			case ch <- cif.CIFStreamStart{Type: "stream_start", ID: "canceled", Model: request.Model}:
			case <-ctx.Done():
				close(providerCanceled)
				return
			}
			<-ctx.Done()
			close(providerCanceled)
		}()
		return ch, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/responses", strings.NewReader(`{"model":"cache-responses-canceled","stream":true,"input":"same"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer test-api-key")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	response.Body.Close()
	select {
	case <-providerCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not observe client cancellation")
	}

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 0 {
		t.Fatalf("canceled stream populated %d entries", stats.Entries)
	}
}

func TestResponsesCachePreservesNativeToolsAndState(t *testing.T) {
	t.Run("custom and function tools", func(t *testing.T) {
		srv, _ := newResponseCacheTestServer(t)
		calls := 0
		rawInput := "*** Begin Patch\n+line one\n+line two\n*** End Patch"
		emptyInput := ""
		registerStubProvider(t, "cache-responses-tools", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			calls++
			return &cif.CanonicalResponse{
				ID:    "tool-response",
				Model: request.Model,
				Content: []cif.CIFContentPart{
					cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_patch", ToolName: "ApplyPatch", ToolArguments: map[string]interface{}{"input": rawInput}, ToolKind: cif.CIFToolKindCustom, RawInput: &rawInput, Namespace: "workspace"},
					cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_empty", ToolName: "ApplyPatch", ToolArguments: map[string]interface{}{"input": emptyInput}, ToolKind: cif.CIFToolKindCustom, RawInput: &emptyInput},
					cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_read", ToolName: "Read", ToolArguments: map[string]interface{}{"path": "a.txt"}},
				},
				StopReason: cif.StopReasonToolUse,
			}, nil
		}, nil)

		requestBody := `{"model":"cache-responses-tools","input":"tools","tools":[{"type":"custom","name":"ApplyPatch","format":{"type":"text"}},{"type":"function","name":"Read","parameters":{"type":"object"}}]}`
		population := postJSON(t, srv.URL+"/v1/responses", requestBody, nil)
		population.Body.Close()
		replay := postJSON(t, srv.URL+"/v1/responses", requestBody, nil)
		body := readBody(t, replay)
		if replay.Header.Get(responsecache.BypassHeader) != "hit" || !strings.Contains(body, "*** Begin Patch") || !strings.Contains(body, `"namespace":"workspace"`) || !strings.Contains(body, `"type":"custom_tool_call"`) || !strings.Contains(body, `"type":"function_call"`) {
			t.Fatalf("nonstream native tool replay invalid: %s", body)
		}
		streamBody := strings.Replace(requestBody, `{"model":`, `{"stream":true,"model":`, 1)
		stream := postJSON(t, srv.URL+"/v1/responses", streamBody, nil)
		streamOutput := readBody(t, stream)
		if stream.Header.Get(responsecache.BypassHeader) != "hit" || strings.Count(streamOutput, "event: response.completed") != 1 || !strings.Contains(streamOutput, "response.custom_tool_call_input.delta") || !strings.Contains(streamOutput, "response.function_call_arguments.delta") || !strings.Contains(streamOutput, "*** Begin Patch") {
			t.Fatalf("stream native tool replay invalid: %s", streamOutput)
		}
		if calls != 1 {
			t.Fatalf("upstream calls=%d, want 1", calls)
		}
	})

	t.Run("stream-origin custom tool", func(t *testing.T) {
		srv, _ := newResponseCacheTestServer(t)
		calls := 0
		const rawInput = "*** Begin Patch\n+line one\n+line two\n*** End Patch"
		registerStubProvider(t, "cache-responses-stream-tools", nil, func(_ context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
			calls++
			ch := make(chan cif.CIFStreamEvent, 6)
			ch <- cif.CIFStreamStart{Type: "stream_start", ID: "stream-tool-response", Model: request.Model}
			ch <- cif.CIFContentDelta{
				Type:  "content_delta",
				Index: 0,
				ContentBlock: cif.CIFToolCallPart{
					Type:       "tool_call",
					ToolCallID: "call_patch_stream",
					ToolName:   "ApplyPatch",
					ToolKind:   cif.CIFToolKindCustom,
					Namespace:  "workspace",
				},
				Delta: cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: "*** Begin Patch\n+line one\n"},
			}
			ch <- cif.CIFContentDelta{Type: "content_delta", Index: 0, Delta: cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: "+line two\n*** End Patch"}}
			ch <- cif.CIFContentBlockStop{Type: "content_block_stop", Index: 0}
			ch <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse, Usage: &cif.CIFUsage{InputTokens: 11, OutputTokens: 5}}
			close(ch)
			return ch, nil
		})

		requestBody := `{"model":"cache-responses-stream-tools","stream":true,"input":"tools","tools":[{"type":"custom","name":"ApplyPatch","format":{"type":"text"}}]}`
		for requestIndex, wantHeader := range []string{"miss", "hit"} {
			response := postJSON(t, srv.URL+"/v1/responses", requestBody, nil)
			body := readBody(t, response)
			if response.StatusCode != http.StatusOK || response.Header.Get(responsecache.BypassHeader) != wantHeader {
				t.Fatalf("request %d status=%d cache=%q body=%s", requestIndex, response.StatusCode, response.Header.Get(responsecache.BypassHeader), body)
			}
			if strings.Count(body, "event: response.completed") != 1 || !strings.Contains(body, "response.custom_tool_call_input.delta") || !strings.Contains(body, "response.custom_tool_call_input.done") {
				t.Fatalf("request %d lost custom event vocabulary: %s", requestIndex, body)
			}
			if strings.Contains(body, "response.function_call_arguments") || !strings.Contains(body, `"namespace":"workspace"`) || !strings.Contains(body, strings.ReplaceAll(rawInput, "\n", `\n`)) {
				t.Fatalf("request %d lost custom metadata or exact input: %s", requestIndex, body)
			}
		}
		if calls != 1 {
			t.Fatalf("upstream stream calls=%d, want 1", calls)
		}
	})

	t.Run("previous response id isolates chains", func(t *testing.T) {
		srv, _ := newResponseCacheTestServer(t)
		calls := 0
		registerStubProvider(t, "cache-responses-state", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			calls++
			chain := "none"
			if request.PreviousResponseID != nil {
				chain = *request.PreviousResponseID
			}
			return &cif.CanonicalResponse{ID: "state-" + chain, Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: chain}}, StopReason: cif.StopReasonEndTurn}, nil
		}, nil)

		for _, chain := range []string{"response-a", "response-b", "response-a", "response-b"} {
			body := fmt.Sprintf(`{"model":"cache-responses-state","previous_response_id":%q,"input":"continue"}`, chain)
			response := postJSON(t, srv.URL+"/v1/responses", body, nil)
			responseBody := readBody(t, response)
			if !strings.Contains(responseBody, chain) {
				t.Fatalf("chain %q received wrong response: %s", chain, responseBody)
			}
		}
		if calls != 2 {
			t.Fatalf("upstream calls=%d, want 2 isolated populations", calls)
		}
	})
}

func TestResponseCacheCorruptEntryFallsThroughWithoutHitAccounting(t *testing.T) {
	srv, store := newResponseCacheTestServer(t)

	model := "cache-corrupt-fallthrough"
	canonicalRequest, err := ingestion.ParseResponsesPayload([]byte(fmt.Sprintf(`{"model":%q,"input":"same"}`, model)))
	if err != nil {
		t.Fatal(err)
	}
	key, err := responsecache.Key(canonicalRequest)
	if err != nil {
		t.Fatal(err)
	}
	store.records[key] = &responsecache.Record{Key: key, ModelID: model, ResponseData: `{not-json`}

	calls := 0
	registerStubProvider(t, model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		calls++
		return &cif.CanonicalResponse{ID: "corrupt-fallback", Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "provider-fallback"}}, StopReason: cif.StopReasonEndTurn}, nil
	}, nil)

	response := postJSON(t, srv.URL+"/v1/responses", fmt.Sprintf(`{"model":%q,"input":"same"}`, model), nil)
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK || response.Header.Get(responsecache.BypassHeader) != "miss" || !strings.Contains(body, "provider-fallback") || calls != 1 {
		t.Fatalf("status=%d cache=%q calls=%d body=%s", response.StatusCode, response.Header.Get(responsecache.BypassHeader), calls, body)
	}
}

func TestResponseCacheHitMeteringSeparatesProviderPromptCache(t *testing.T) {
	srv, _ := newResponseCacheTestServer(t)

	model := "cache-metering-model"
	cacheRead := 5
	cacheWrite := 3
	providerID := registerStubProvider(t, model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		return &cif.CanonicalResponse{
			ID:         "metered-cache",
			Model:      request.Model,
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "metered"}},
			StopReason: cif.StopReasonEndTurn,
			Usage:      &cif.CIFUsage{InputTokens: 20, OutputTokens: 4, CacheReadInputTokens: &cacheRead, CacheWriteInputTokens: &cacheWrite},
		}, nil
	}, nil)

	requestBody := fmt.Sprintf(`{"model":%q,"input":"same"}`, model)
	for range 2 {
		response := postJSON(t, srv.URL+"/v1/responses", requestBody, map[string]string{"User-Agent": "cache-test/1.0"})
		response.Body.Close()
	}

	database.StopAsyncWorkers()
	t.Cleanup(database.StartAsyncWorkers)
	records, count, err := database.GetDatabase().ListMeteringRecords(database.MeteringFilter{ModelID: model}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(records) != 2 {
		t.Fatalf("metering rows=%d count=%d, want 2", len(records), count)
	}
	byProvider := make(map[string]database.MeteringRecord, len(records))
	for _, record := range records {
		byProvider[record.ProviderID] = record
	}
	miss := byProvider[providerID]
	if miss.CacheReadInputTokens == nil || *miss.CacheReadInputTokens != cacheRead || miss.CacheWriteInputTokens == nil || *miss.CacheWriteInputTokens != cacheWrite || miss.PromptCacheStatus != database.PromptCacheHit {
		t.Fatalf("provider miss metering lost prompt-cache detail: %#v", miss)
	}
	hit := byProvider["response-cache"]
	if hit.InputTokens != 20 || hit.OutputTokens != 4 || hit.TotalTokens != 24 || hit.CacheReadInputTokens != nil || hit.CacheWriteInputTokens != nil || hit.CacheWrite5mInputTokens != nil || hit.CacheWrite1hInputTokens != nil || hit.UncachedInputTokens != nil || hit.PromptCacheStatus != database.PromptCacheUnknown {
		t.Fatalf("served-hit metering conflated provider prompt cache: %#v", hit)
	}
}

func TestResponseCacheAdminDegradedAndClearFailure(t *testing.T) {
	restore := responsecache.ConfigureStore(nil)
	defer restore()
	srv := newTestServer(t)
	defer srv.Close()

	response := getWithAuth(t, srv.URL+"/api/admin/settings/response-cache")
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("settings status %d: %s", response.StatusCode, body)
	}
	var status struct {
		Backend   string `json:"backend"`
		Available bool   `json:"available"`
		Entries   int64  `json:"entries"`
		TotalHits int64  `json:"total_hits"`
	}
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if status.Backend != "redis" || status.Available || status.Entries != 0 || status.TotalHits != 0 {
		t.Fatalf("unexpected degraded status: %+v", status)
	}

	request, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/admin/settings/response-cache", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-api-key")
	clearResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("clear request: %v", err)
	}
	defer clearResponse.Body.Close()
	if clearResponse.StatusCode != http.StatusInternalServerError {
		t.Fatalf("clear status = %d", clearResponse.StatusCode)
	}
}
