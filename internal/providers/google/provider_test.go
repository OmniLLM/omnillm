package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "google-provider-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	if err := database.InitializeDatabase(dir); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func ptr[T any](v T) *T { return &v }

// ─── SetupAuth ───────────────────────────────────────────────────────────────

func TestSetupAuthRequiresAPIKey(t *testing.T) {
	_, _, _, err := SetupAuth("google-1", &types.AuthOptions{})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestSetupAuthSetsCorrectValues(t *testing.T) {
	token, baseURL, name, err := SetupAuth("google-test-1", &types.AuthOptions{
		APIKey: "AIza-test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "AIza-test-key" {
		t.Errorf("token = %q", token)
	}
	if baseURL != "https://generativelanguage.googleapis.com" {
		t.Errorf("baseURL = %q", baseURL)
	}
	if name == "" {
		t.Error("name should not be empty")
	}

	// Verify persisted
	store := database.NewTokenStore()
	rec, err := store.Get("google-test-1")
	if err != nil || rec == nil {
		t.Fatalf("token not persisted: err=%v", err)
	}
}

// ─── Headers ─────────────────────────────────────────────────────────────────

func TestGoogleHeaders(t *testing.T) {
	h := Headers("AIza-my-key")
	if h["x-goog-api-key"] != "AIza-my-key" {
		t.Errorf("x-goog-api-key = %q", h["x-goog-api-key"])
	}
	if h["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", h["Content-Type"])
	}
}

// ─── StreamURL ───────────────────────────────────────────────────────────────

func TestStreamURL(t *testing.T) {
	url := StreamURL("https://generativelanguage.googleapis.com", "gemini-2.5-flash")
	want := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse"
	if url != want {
		t.Errorf("got %q, want %q", url, want)
	}
}

func TestStreamURLFallsBackToDefault(t *testing.T) {
	url := StreamURL("", "gemini-2.5-flash")
	if url == "" {
		t.Error("URL should not be empty")
	}
	if !containsString(url, "generativelanguage.googleapis.com") {
		t.Errorf("URL %q should contain default base", url)
	}
}

// ─── FetchModels ─────────────────────────────────────────────────────────────

func TestFetchModelsFiltersToGenerateContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"name":                       "models/gemini-2.5-flash",
					"displayName":                "Gemini 2.5 Flash",
					"outputTokenLimit":           65536,
					"supportedGenerationMethods": []string{"generateContent", "countTokens"},
				},
				{
					"name":                       "models/text-embedding-004",
					"displayName":                "Text Embedding 004",
					"outputTokenLimit":           0,
					"supportedGenerationMethods": []string{"embedContent"},
				},
				{
					"name":                       "models/gemini-2.5-pro",
					"displayName":                "Gemini 2.5 Pro",
					"outputTokenLimit":           65536,
					"supportedGenerationMethods": []string{"generateContent"},
				},
			},
		})
	}))
	defer srv.Close()

	resp, err := FetchModels("google-1", "test-token", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only models with generateContent should be included
	if len(resp.Data) != 2 {
		t.Fatalf("got %d models, want 2: %v", len(resp.Data), modelIDs(resp.Data))
	}
	for _, m := range resp.Data {
		if m.Provider != "google-1" {
			t.Errorf("model %q has provider %q, want google-1", m.ID, m.Provider)
		}
	}
}

func TestFetchModelsStripsModelsPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"name":                       "models/gemini-2.5-flash",
					"displayName":                "Gemini 2.5 Flash",
					"outputTokenLimit":           65536,
					"supportedGenerationMethods": []string{"generateContent"},
				},
			},
		})
	}))
	defer srv.Close()

	resp, err := FetchModels("google-1", "test-token", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "gemini-2.5-flash" {
		t.Errorf("ID = %q, want gemini-2.5-flash (models/ prefix should be stripped)", resp.Data[0].ID)
	}
}

func TestFetchModelsDefaultsMaxTokensWhenZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"name":                       "models/gemini-1.5-flash",
					"displayName":                "Gemini 1.5 Flash",
					"outputTokenLimit":           0, // missing/zero
					"supportedGenerationMethods": []string{"generateContent"},
				},
			},
		})
	}))
	defer srv.Close()

	resp, err := FetchModels("google-1", "test-token", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data[0].MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192 (default)", resp.Data[0].MaxTokens)
	}
}

func TestFetchModelsRequiresToken(t *testing.T) {
	_, err := FetchModels("google-1", "", "https://example.com")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestFetchModelsHandlesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "API key expired", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := FetchModels("google-1", "bad-key", srv.URL)
	if err == nil {
		t.Error("expected error for API failure")
	}
}

// ─── StopReason ──────────────────────────────────────────────────────────────

func TestGoogleStopReason(t *testing.T) {
	cases := []struct {
		reason string
		want   cif.CIFStopReason
	}{
		{"STOP", cif.StopReasonEndTurn},
		{"MAX_TOKENS", cif.StopReasonMaxTokens},
		{"FUNCTION_CALL", cif.StopReasonToolUse},
		{"SAFETY", cif.StopReasonContentFilter},
		{"RECITATION", cif.StopReasonContentFilter},
		{"LANGUAGE", cif.StopReasonContentFilter},
		{"UNKNOWN", cif.StopReasonEndTurn},
		{"", cif.StopReasonEndTurn},
	}
	for _, tc := range cases {
		got := StopReason(tc.reason)
		if got != tc.want {
			t.Errorf("StopReason(%q) = %v, want %v", tc.reason, got, tc.want)
		}
	}
}

// ─── BuildPayload ─────────────────────────────────────────────────────────────

func TestBuildPayloadSystemInstruction(t *testing.T) {
	request := &cif.CanonicalRequest{
		Model: "gemini-2.5-flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "Hello"},
			}},
		},
		SystemPrompt: ptr("You are helpful."),
	}
	payload := BuildPayload("gemini-2.5-flash", request)

	if payload["systemInstruction"] == nil {
		t.Error("expected systemInstruction")
	}
	if payload["contents"] == nil {
		t.Error("expected contents")
	}
}

func TestBuildPayloadWithTools(t *testing.T) {
	desc := "Search the web"
	request := &cif.CanonicalRequest{
		Model:    "gemini-2.5-flash",
		Messages: []cif.CIFMessage{},
		Tools: []cif.CIFTool{
			{
				Name:             "web_search",
				Description:      &desc,
				ParametersSchema: map[string]interface{}{"type": "object"},
			},
		},
	}
	payload := BuildPayload("gemini-2.5-flash", request)
	tools, ok := payload["tools"].([]map[string]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("expected tools in payload")
	}
	decls, ok := tools[0]["functionDeclarations"].([]map[string]interface{})
	if !ok || len(decls) == 0 {
		t.Fatal("expected functionDeclarations")
	}
	if decls[0]["name"] != "web_search" {
		t.Errorf("tool name = %v", decls[0]["name"])
	}
}

// ─── ParseGeminiSSE ──────────────────────────────────────────────────────────

func TestParseGeminiSSETextStream(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"}}]}
data: {"candidates":[{"content":{"parts":[{"text":" world"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3}}
`
	eventCh := make(chan cif.CIFStreamEvent, 16)
	go ParseGeminiSSE(newReadCloser(sseData), eventCh)

	var events []cif.CIFStreamEvent
	for event := range eventCh {
		events = append(events, event)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	if _, ok := events[0].(cif.CIFStreamStart); !ok {
		t.Errorf("first event = %T, want CIFStreamStart", events[0])
	}

	// Find text deltas
	var textDeltaTexts []string
	for _, e := range events {
		if delta, ok := e.(cif.CIFContentDelta); ok {
			if td, ok := delta.Delta.(cif.TextDelta); ok {
				textDeltaTexts = append(textDeltaTexts, td.Text)
			}
		}
	}
	if len(textDeltaTexts) == 0 {
		t.Error("expected at least one text delta")
	}

	last := events[len(events)-1]
	end, ok := last.(cif.CIFStreamEnd)
	if !ok {
		t.Errorf("last event = %T, want CIFStreamEnd", last)
	}
	if end.StopReason != cif.StopReasonEndTurn {
		t.Errorf("stop reason = %v, want end_turn", end.StopReason)
	}
	if end.Usage == nil || end.Usage.InputTokens != 5 || end.Usage.OutputTokens != 3 {
		t.Errorf("usage = %v", end.Usage)
	}
}

func TestParseGeminiSSEToolCall(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"search","args":{"q":"test"}}}],"role":"model"},"finishReason":"FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}
`
	eventCh := make(chan cif.CIFStreamEvent, 16)
	go ParseGeminiSSE(newReadCloser(sseData), eventCh)

	var events []cif.CIFStreamEvent
	for event := range eventCh {
		events = append(events, event)
	}

	// Find tool call delta
	var foundToolCall bool
	for _, e := range events {
		if delta, ok := e.(cif.CIFContentDelta); ok {
			if _, ok := delta.Delta.(cif.ToolArgumentsDelta); ok {
				foundToolCall = true
				if tc, ok := delta.ContentBlock.(cif.CIFToolCallPart); ok {
					if tc.ToolName != "search" {
						t.Errorf("ToolName = %q, want search", tc.ToolName)
					}
				}
				break
			}
		}
	}
	if !foundToolCall {
		t.Fatal("expected tool arguments delta")
	}

	// Check stop reason
	last := events[len(events)-1]
	if end, ok := last.(cif.CIFStreamEnd); ok {
		if end.StopReason != cif.StopReasonToolUse {
			t.Errorf("stop reason = %v, want tool_use", end.StopReason)
		}
	} else {
		t.Errorf("last event = %T, want CIFStreamEnd", last)
	}
}

func TestParseGeminiSSESafetyFilter(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"SAFETY"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":0}}
`
	eventCh := make(chan cif.CIFStreamEvent, 16)
	go ParseGeminiSSE(newReadCloser(sseData), eventCh)

	var events []cif.CIFStreamEvent
	for event := range eventCh {
		events = append(events, event)
	}

	last := events[len(events)-1]
	if end, ok := last.(cif.CIFStreamEnd); ok {
		if end.StopReason != cif.StopReasonContentFilter {
			t.Errorf("stop reason = %v, want content_filter", end.StopReason)
		}
	} else {
		t.Errorf("last event = %T, want CIFStreamEnd", last)
	}
}

// ─── Execute end-to-end ───────────────────────────────────────────────────────

func TestExecuteEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if r.Header.Get("x-goog-api-key") != "test-api-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", mustMarshal(map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{{"text": "Hello world"}},
						"role":  "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     10,
				"candidatesTokenCount": 5,
			},
		}))
	}))
	defer srv.Close()

	request := &cif.CanonicalRequest{
		Model: "gemini-2.5-flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "Hello"},
			}},
		},
	}

	resp, err := Execute(context.Background(), "test-api-key", srv.URL, request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(resp.Content))
	}
	textPart, ok := resp.Content[0].(cif.CIFTextPart)
	if !ok {
		t.Fatalf("expected CIFTextPart, got %T", resp.Content[0])
	}
	if textPart.Text != "Hello world" {
		t.Errorf("text = %q, want Hello world", textPart.Text)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 10 {
		t.Errorf("usage = %v", resp.Usage)
	}
}

func TestExecuteRequiresAuth(t *testing.T) {
	request := &cif.CanonicalRequest{
		Model:    "gemini-2.5-flash",
		Messages: []cif.CIFMessage{},
	}
	_, err := Execute(context.Background(), "", "https://example.com", request)
	if err == nil {
		t.Error("expected error for empty token")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func modelIDs(models []types.Model) []string {
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
}

type readCloser struct {
	*strings.Reader
}

func (r readCloser) Close() error { return nil }

func newReadCloser(s string) readCloser {
	return readCloser{strings.NewReader(s)}
}

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
