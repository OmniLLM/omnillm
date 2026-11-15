package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

// ─── RemapModel ───────────────────────────────────────────────────────────────

func TestRemapModel(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{"claude-opus-4-beta", "claude-opus-4-6-thinking"},
		{"claude-opus-4-6-thinking", "claude-opus-4-6-thinking"},
		{"claude-sonnet-4-latest", "claude-sonnet-4-6"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"claude-haiku-4-mini", "gemini-3-flash"},
		{"gemini-3-flash", "gemini-3-flash"},
		{"gemini-2.5-flash", "gemini-2.5-flash"},
		{"gpt-oss-120b-medium", "gpt-oss-120b-medium"},
	}
	for _, tc := range cases {
		got := RemapModel(tc.model)
		if got != tc.want {
			t.Errorf("RemapModel(%q) = %q, want %q", tc.model, got, tc.want)
		}
	}
}

// ─── GetModels ────────────────────────────────────────────────────────────────

func TestGetModels(t *testing.T) {
	resp := GetModels("antigravity-1")
	if len(resp.Data) != len(Models) {
		t.Errorf("got %d models, want %d", len(resp.Data), len(Models))
	}
	for _, m := range resp.Data {
		if m.Provider != "antigravity-1" {
			t.Errorf("model %q has provider %q, want antigravity-1", m.ID, m.Provider)
		}
	}
}

func TestStreamBuildsConservativeGeminiPayload(t *testing.T) {
	var gotAgent string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotAgent = r.Header.Get("User-Agent")
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	maxTokens := 128
	request := &cif.CanonicalRequest{
		Model:     "claude-sonnet-4-6",
		Messages:  []cif.CIFMessage{cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ping"}}}},
		MaxTokens: &maxTokens,
		Tools: []cif.CIFTool{{
			Name: "Read",
		}},
	}

	ch, err := Stream(t.Context(), "token", server.URL, "", request)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	for range ch {
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if gotAgent != "antigravity/2.6.0" || payload["userAgent"] != gotAgent {
		t.Fatalf("client identity mismatch: header=%q body=%v", gotAgent, payload["userAgent"])
	}
	if payload["model"] != request.Model {
		t.Fatal("model changed")
	}
	requestMap, ok := payload["request"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing request envelope: %#v", payload)
	}
	if _, ok := requestMap["generationConfig"].(map[string]interface{}); !ok {
		t.Fatalf("missing generationConfig: %#v", requestMap)
	}
	tools, ok := requestMap["tools"].([]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("unexpected tools payload: %#v", requestMap["tools"])
	}
	toolGroup, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected tool group: %#v", tools[0])
	}
	decls, ok := toolGroup["functionDeclarations"].([]interface{})
	if !ok || len(decls) != 1 {
		t.Fatalf("unexpected functionDeclarations: %#v", toolGroup["functionDeclarations"])
	}
	decl, ok := decls[0].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected declaration: %#v", decls[0])
	}
	if _, ok := decl["parameters"]; ok {
		t.Fatal("expected nil tool schema to omit parameters field")
	}
	if _, ok := decl["parametersJsonSchema"]; ok {
		t.Fatal("expected nil tool schema to omit parametersJsonSchema field")
	}
	if decl["name"] != "Read" {
		t.Fatalf("unexpected tool name: %#v", decl["name"])
	}
}

// ─── StopReason ───────────────────────────────────────────────────────────────

func TestStopReason(t *testing.T) {
	cases := []struct {
		reason string
		want   cif.CIFStopReason
	}{
		{"STOP", cif.StopReasonEndTurn},
		{"MAX_TOKENS", cif.StopReasonMaxTokens},
		{"FUNCTION_CALL", cif.StopReasonToolUse},
		{"SAFETY", cif.StopReasonContentFilter},
		{"RECITATION", cif.StopReasonContentFilter},
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

// ─── ParseAntigravitySSE text stream ─────────────────────────────────────────

func TestParseAntigravitySSETextStream(t *testing.T) {
	// Antigravity wraps Gemini in a "response" envelope
	sseData := buildSSEEvent(map[string]interface{}{
		"response": map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{{"text": "Hello "}},
						"role":  "model",
					},
				},
			},
		},
	}) + buildSSEEvent(map[string]interface{}{
		"response": map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{{"text": "world"}},
						"role":  "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     8,
				"candidatesTokenCount": 2,
			},
		},
	})

	eventCh := make(chan cif.CIFStreamEvent, 16)
	go ParseAntigravitySSE(context.Background(), newReadCloser(sseData), eventCh)

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

	var textParts []string
	for _, e := range events {
		if delta, ok := e.(cif.CIFContentDelta); ok {
			if td, ok := delta.Delta.(cif.TextDelta); ok {
				textParts = append(textParts, td.Text)
			}
		}
	}
	assembled := strings.Join(textParts, "")
	if assembled != "Hello world" {
		t.Errorf("assembled text = %q, want %q", assembled, "Hello world")
	}

	last := events[len(events)-1]
	end, ok := last.(cif.CIFStreamEnd)
	if !ok {
		t.Fatalf("last event = %T, want CIFStreamEnd", last)
	}
	if end.StopReason != cif.StopReasonEndTurn {
		t.Errorf("stop reason = %v, want end_turn", end.StopReason)
	}
	if end.Usage == nil || end.Usage.InputTokens != 8 || end.Usage.OutputTokens != 2 {
		t.Errorf("usage = %v", end.Usage)
	}
}

// ─── ParseAntigravitySSE tool call ────────────────────────────────────────────

func TestParseAntigravitySSEToolCall(t *testing.T) {
	sseData := buildSSEEvent(map[string]interface{}{
		"response": map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"functionCall": map[string]interface{}{
								"name": "search",
								"args": map[string]interface{}{"q": "golang"},
							}},
						},
						"role": "model",
					},
					"finishReason": "FUNCTION_CALL",
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     12,
				"candidatesTokenCount": 8,
			},
		},
	})

	eventCh := make(chan cif.CIFStreamEvent, 16)
	go ParseAntigravitySSE(context.Background(), newReadCloser(sseData), eventCh)

	var events []cif.CIFStreamEvent
	for event := range eventCh {
		events = append(events, event)
	}

	// Find tool call delta
	var foundTool bool
	for _, e := range events {
		if delta, ok := e.(cif.CIFContentDelta); ok {
			if _, ok := delta.Delta.(cif.ToolArgumentsDelta); ok {
				foundTool = true
				if tc, ok := delta.ContentBlock.(cif.CIFToolCallPart); ok {
					if tc.ToolName != "search" {
						t.Errorf("ToolName = %q, want search", tc.ToolName)
					}
				}
				break
			}
		}
	}
	if !foundTool {
		t.Fatal("expected tool call delta")
	}

	last := events[len(events)-1]
	if end, ok := last.(cif.CIFStreamEnd); ok {
		if end.StopReason != cif.StopReasonToolUse {
			t.Errorf("stop reason = %v, want tool_use", end.StopReason)
		}
	} else {
		t.Errorf("last event = %T, want CIFStreamEnd", last)
	}
}

// ─── CollectStream via Execute ─────────────────────────────────────────────────

func TestCollectStreamFromSSE(t *testing.T) {
	// Directly test ParseAntigravitySSE + CollectStream integration via shared.CollectStream
	sseData := buildSSEEvent(map[string]interface{}{
		"response": map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{{"text": "Hello!"}},
						"role":  "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     3,
				"candidatesTokenCount": 1,
			},
		},
	})

	eventCh := make(chan cif.CIFStreamEvent, 16)
	go ParseAntigravitySSE(context.Background(), newReadCloser(sseData), eventCh)

	// Manually collect
	var textBuf strings.Builder
	var stopReason cif.CIFStopReason
	var usage *cif.CIFUsage
	for event := range eventCh {
		switch e := event.(type) {
		case cif.CIFContentDelta:
			if td, ok := e.Delta.(cif.TextDelta); ok {
				textBuf.WriteString(td.Text)
			}
		case cif.CIFStreamEnd:
			stopReason = e.StopReason
			usage = e.Usage
		}
	}

	if textBuf.String() != "Hello!" {
		t.Errorf("text = %q, want Hello!", textBuf.String())
	}
	if stopReason != cif.StopReasonEndTurn {
		t.Errorf("stop reason = %v", stopReason)
	}
	if usage == nil || usage.InputTokens != 3 {
		t.Errorf("usage = %v", usage)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

type readCloser struct {
	*strings.Reader
}

func (r readCloser) Close() error { return nil }

func newReadCloser(s string) readCloser {
	return readCloser{strings.NewReader(s)}
}

func buildSSEEvent(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("buildSSEEvent: %v", err))
	}
	return "data: " + string(b) + "\n\n"
}
