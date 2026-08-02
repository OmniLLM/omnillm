package copilot

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"omnillm/internal/cif"
)

func TestChatCompletionsClientUsesClaudeHeaderBudget(t *testing.T) {
	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)

	client, budget := adapter.chatCompletionsClient("owner/claude-opus-5", true)
	if client != copilotClaudeClient {
		t.Fatal("Claude model did not select dedicated header client")
	}
	if budget != copilotClaudeHeaderTimeout {
		t.Fatalf("Claude budget = %v, want %v", budget, copilotClaudeHeaderTimeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Claude transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout != copilotClaudeHeaderTimeout {
		t.Fatalf("response header timeout = %v, want %v", transport.ResponseHeaderTimeout, copilotClaudeHeaderTimeout)
	}
	if client.Timeout != 0 {
		t.Fatalf("Claude total timeout = %v, want 0 so SSE bodies remain unbounded", client.Timeout)
	}
}

func TestChatCompletionsClientBoundsNonStreamingClaudeBody(t *testing.T) {
	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)

	client, budget := adapter.chatCompletionsClient("owner/claude-opus-5", false)
	if client != copilotClaudeBoundedClient {
		t.Fatal("non-streaming Claude model did not select bounded client")
	}
	if client.Timeout != budget || budget != copilotClaudeHeaderTimeout {
		t.Fatalf("non-streaming Claude total timeout = %v, budget = %v, want %v", client.Timeout, budget, copilotClaudeHeaderTimeout)
	}
}

func TestChatCompletionsClientPreservesOrdinaryBudget(t *testing.T) {
	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)

	client, budget := adapter.chatCompletionsClient("gpt-4o", false)
	if client != copilotHTTPClient {
		t.Fatal("ordinary model did not select the ordinary Copilot client")
	}
	if budget != copilotHTTPClient.Timeout {
		t.Fatalf("ordinary budget = %v, want %v", budget, copilotHTTPClient.Timeout)
	}
	if copilotClaudeHeaderTimeout <= copilotHTTPClient.Timeout {
		t.Fatalf("Claude budget (%v) must exceed ordinary budget (%v)", copilotClaudeHeaderTimeout, copilotHTTPClient.Timeout)
	}
}

func TestChatCompletionsStreamClientPreservesUnboundedBody(t *testing.T) {
	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)

	client, budget := adapter.chatCompletionsClient("gpt-4o", true)
	if client != copilotChatStreamClient {
		t.Fatal("ordinary streaming model did not select the header-only timeout client")
	}
	if budget != copilotHTTPClient.Timeout {
		t.Fatalf("stream header budget = %v, want %v", budget, copilotHTTPClient.Timeout)
	}
	if client.Timeout != 0 {
		t.Fatalf("stream total timeout = %v, want 0", client.Timeout)
	}
}

func TestCopilotTimeoutLogIncludesCorrelationFields(t *testing.T) {
	var output strings.Builder
	logger := zerolog.New(&output).With().Str("request_id", "request-123").Logger()
	ctx := logger.WithContext(context.Background())

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)
	request := &cif.CanonicalRequest{Model: "owner/claude-opus-5"}

	adapter.logUpstreamTimeout(ctx, request, "chat.completions-stream", 5*time.Minute, 2*time.Minute, context.DeadlineExceeded)

	logged := output.String()
	for _, field := range []string{
		`"request_id":"request-123"`,
		`"provider":"github-copilot-test"`,
		`"endpoint":"chat.completions-stream"`,
		`"model":"owner/claude-opus-5"`,
		`"timeout_budget_ms":300000`,
		`"elapsed_ms":120000`,
	} {
		if !strings.Contains(logged, field) {
			t.Errorf("timeout log missing %s: %s", field, logged)
		}
	}
}

func TestCopilotTimeoutLogIgnoresNonTimeout(t *testing.T) {
	var output strings.Builder
	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)
	adapter.logUpstreamTimeout(ctx, nil, "chat.completions", time.Minute, time.Second, errors.New("connection refused"))
	if output.Len() != 0 {
		t.Fatalf("non-timeout error produced timeout log: %s", output.String())
	}
}
