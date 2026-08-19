package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/translation/toolarguments"
)

func claudeCodeAgentRequestBody(t *testing.T, model string, stream bool) string {
	t.Helper()
	return mustMarshalJSON(t, map[string]interface{}{
		"model": model, "max_tokens": 1024, "stream": stream,
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "plan this"}},
		"tools": []interface{}{map[string]interface{}{
			"name": "Agent", "description": "launch agent",
			"input_schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"subagent_type": map[string]interface{}{"type": "string", "enum": []interface{}{"Explore", "Plan"}},
					"prompt":        map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"subagent_type", "prompt"},
			},
		}},
	})
}

func planAgentResponse(model string) *cif.CanonicalResponse {
	return &cif.CanonicalResponse{
		ID: "msg_plan", Model: model, StopReason: cif.StopReasonToolUse,
		Content: []cif.CIFContentPart{cif.CIFToolCallPart{
			Type: "tool_call", ToolCallID: "call_plan", ToolName: "Agent",
			ToolArguments: map[string]interface{}{
				"subagent_type": toolarguments.ClaudeCodePlanAgentTypeSentinel,
				"prompt":        "keep this prompt",
			},
		}},
	}
}

func TestClaudeCodePlanAgentTypeCompatibilityNonStreaming(t *testing.T) {
	model := "claude-code-plan-compat-nonstream"
	registerStubProvider(t, model, func(_ context.Context, _ *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		return planAgentResponse(model), nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	rows := []struct {
		name      string
		userAgent string
		want      string
	}{
		{"claude-code", "claude-cli/2.1.235 (external, cli)", "Plan"},
		{"other-client", "codex-tui/0.148.0", toolarguments.ClaudeCodePlanAgentTypeSentinel},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/v1/messages", claudeCodeAgentRequestBody(t, model, false), map[string]string{
				"anthropic-version": "2023-06-01", "User-Agent": row.userAgent,
			})
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d: %s", resp.StatusCode, body)
			}
			var result struct {
				Content []struct {
					Type  string                 `json:"type"`
					Input map[string]interface{} `json:"input"`
				} `json:"content"`
			}
			if err := json.Unmarshal([]byte(body), &result); err != nil || len(result.Content) != 1 {
				t.Fatalf("decode response: %v: %s", err, body)
			}
			if got := result.Content[0].Input["subagent_type"]; got != row.want || result.Content[0].Input["prompt"] != "keep this prompt" {
				t.Fatalf("tool input = %#v, want subagent_type %q", result.Content[0].Input, row.want)
			}
		})
	}
}

func TestClaudeCodePlanAgentTypeCompatibilityStreaming(t *testing.T) {
	model := "claude-code-plan-compat-stream"
	registerStubProvider(t, model, nil, func(_ context.Context, _ *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
		events := make(chan cif.CIFStreamEvent, 8)
		events <- cif.CIFStreamStart{Type: "stream_start", ID: "msg_stream", Model: model}
		events <- cif.CIFContentDelta{Type: "content_delta", Index: 0, ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_plan", ToolName: "Agent"}}
		cut := len(toolarguments.ClaudeCodePlanAgentTypeSentinel) / 2
		events <- cif.CIFContentDelta{Type: "content_delta", Index: 0, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"subagent_type":"` + toolarguments.ClaudeCodePlanAgentTypeSentinel[:cut]}}
		events <- cif.CIFContentDelta{Type: "content_delta", Index: 0, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: toolarguments.ClaudeCodePlanAgentTypeSentinel[cut:] + `","prompt":"keep this prompt"}`}}
		events <- cif.CIFContentBlockStop{Type: "content_block_stop", Index: 0}
		events <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse}
		close(events)
		return events, nil
	})
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/v1/messages", claudeCodeAgentRequestBody(t, model, true), map[string]string{
		"anthropic-version": "2023-06-01", "User-Agent": "claude-cli/2.1.235 (external, cli)",
	})
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d content-type=%q body=%s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
	if !strings.Contains(body, `\"subagent_type\":\"Plan\"`) || !strings.Contains(body, `\"prompt\":\"keep this prompt\"`) {
		t.Fatalf("repaired streaming input missing: %s", body)
	}
	if strings.Contains(body, "scriptPath not found") {
		t.Fatalf("sentinel survived streaming repair: %s", body)
	}
}
