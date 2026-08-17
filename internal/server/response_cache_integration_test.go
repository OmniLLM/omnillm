package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/database"
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
	records map[string]*responsecache.Record
	hits    int64
}

func newMemoryResponseCacheStore() *memoryResponseCacheStore {
	return &memoryResponseCacheStore{records: make(map[string]*responsecache.Record)}
}

func (s *memoryResponseCacheStore) Get(_ context.Context, key string) (*responsecache.Record, error) {
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
	s.records[key] = &responsecache.Record{Key: key, ModelID: modelID, ResponseData: data}
	return nil
}
func (s *memoryResponseCacheStore) Stats(context.Context) (responsecache.Stats, error) {
	return responsecache.Stats{Entries: int64(len(s.records)), TotalHits: s.hits}, nil
}
func (s *memoryResponseCacheStore) Clear(context.Context) (int64, error) {
	count := int64(len(s.records))
	clear(s.records)
	return count, nil
}
func (s *memoryResponseCacheStore) Available() bool { return true }
func (s *memoryResponseCacheStore) Close() error    { return nil }

func TestResponseCachePopulatesHitsBypassAndOff(t *testing.T) {
	enableResponseCacheForTest(t)
	store := newMemoryResponseCacheStore()
	restore := responsecache.ConfigureStore(store)
	defer restore()

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
	srv := newTestServer(t)
	defer srv.Close()

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
	enableResponseCacheForTest(t)
	store := newMemoryResponseCacheStore()
	restore := responsecache.ConfigureStore(store)
	defer restore()

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
	srv := newTestServer(t)
	defer srv.Close()

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

func TestResponseCacheFailureFallsThroughChatAndMessages(t *testing.T) {
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
			body:     `{"model":"cache-fail-open-model","temperature":0,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`,
			headers:  map[string]string{"anthropic-version": "2023-06-01"},
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
