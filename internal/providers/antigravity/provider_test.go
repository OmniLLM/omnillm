package antigravity

import (
	"encoding/json"
	"fmt"
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
	go ParseAntigravitySSE(newReadCloser(sseData), eventCh)

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
	go ParseAntigravitySSE(newReadCloser(sseData), eventCh)

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
	go ParseAntigravitySSE(newReadCloser(sseData), eventCh)

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
