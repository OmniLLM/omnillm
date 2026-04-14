package server

import (
	"strings"
	"testing"
)

func TestFormatBroadcastLogLine_NormalizesStructuredFields(t *testing.T) {
	line := formatBroadcastLogLine("backend", `{"level":"info","request_id":"abc123","api_shape":"anthropic","model_requested":"claude-sonnet-4-5","model_used":"claude-sonnet-4.5","provider":"github-copilot-main","input_tokens":12,"output_tokens":34,"latency_ms":456,"time":"2026-04-10T20:31:58+08:00","message":"<-- RESPONSE stream"}`)

	wantParts := []string{
		"[2026-04-10T20:31:58+08:00]",
		"backend",
		"INFO",
		"<-- RESPONSE stream",
		"request=abc123",
		"api=anthropic",
		"requested=claude-sonnet-4-5",
		"used=claude-sonnet-4.5",
		"provider=github-copilot-main",
		"input=12",
		"output=34",
		"latency=456ms",
	}

	for _, want := range wantParts {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q in %q", want, line)
		}
	}

	if strings.Contains(line, "time=") {
		t.Fatalf("did not expect raw time field in %q", line)
	}
}

func TestFormatBroadcastLogLine_FallsBackForPlainText(t *testing.T) {
	line := formatBroadcastLogLine("backend", "plain startup line")

	if !strings.Contains(line, " | backend | INFO | plain startup line") {
		t.Fatalf("unexpected fallback line: %q", line)
	}
}

func TestFormatBroadcastLogLine_SuppressesRedundantModelUsed(t *testing.T) {
	line := formatBroadcastLogLine("backend", `{"level":"info","model_requested":"qwen3.6-plus","model_used":"qwen3.6-plus","message":"<-- RESPONSE"}`)

	if strings.Contains(line, "used=") {
		t.Fatalf("expected used= to be suppressed when equal to requested=, got: %q", line)
	}

	if !strings.Contains(line, "requested=qwen3.6-plus") {
		t.Fatalf("expected requested= to still be present, got: %q", line)
	}
}

func TestFormatBroadcastLogLine_SuppressesZeroTokens(t *testing.T) {
	line := formatBroadcastLogLine("backend", `{"level":"info","model_requested":"qwen3.6-plus","input_tokens":0,"output_tokens":0,"message":"<-- RESPONSE"}`)

	if strings.Contains(line, "input=") || strings.Contains(line, "output=") {
		t.Fatalf("expected zero token fields to be suppressed, got: %q", line)
	}
}
