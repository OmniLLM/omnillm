package copilot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"omnillm/internal/cif"
)

// withResponsesBudget temporarily shrinks the /responses client timeout so
// timeout behavior can be exercised in milliseconds rather than minutes.
func withResponsesBudget(t *testing.T, budget time.Duration) {
	t.Helper()
	prev := copilotResponsesClient.Timeout
	copilotResponsesClient.Timeout = budget
	t.Cleanup(func() { copilotResponsesClient.Timeout = prev })
}

func newTestAdapter(t *testing.T, baseURL string) *CopilotAdapter {
	t.Helper()
	provider := NewGitHubCopilotProvider("test", "")
	provider.baseURL = baseURL
	provider.token = "test-token"
	return &CopilotAdapter{provider: provider}
}

func testRequest() *cif.CanonicalRequest {
	return &cif.CanonicalRequest{
		Model: "gpt-5.6-sol",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hello"}},
			},
		},
	}
}

// End-to-end reproduction of the original incident.
//
// An upstream that accepts the connection and then never writes response
// headers previously cost TWO full client budgets before failover: the request
// burned its timeout, was unconditionally retried, and burned it again —
// producing the observed latency=240029ms against a 120s cap.
//
// The upstream must now be dialed exactly once, and total wall-clock must stay
// within roughly one budget.
func TestResponses_StalledUpstreamIsNotRetried(t *testing.T) {
	const budget = 300 * time.Millisecond
	withResponsesBudget(t, budget)

	var attempts int64
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		// Accept the request, then stall without ever writing headers.
		<-release
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	adapter := newTestAdapter(t, server.URL)

	start := time.Now()
	_, err := adapter.executeResponses(context.Background(), testRequest())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error from the stalled upstream")
	}
	if got := atomic.LoadInt64(&attempts); got != 1 {
		t.Fatalf("upstream was called %d times, want exactly 1 (a retry doubles wall-clock before failover)", got)
	}
	// One budget plus scheduling slack — decisively less than the two budgets
	// the old code spent.
	if elapsed > 2*budget {
		t.Fatalf("request took %v, want under %v (~one budget)", elapsed, 2*budget)
	}
	t.Logf("stalled upstream failed after %v with %d attempt(s)", elapsed, atomic.LoadInt64(&attempts))
}

// The retry path survives for transport-level timeouts that fail fast —
// e.g. ResponseHeaderTimeout, dial and TLS timeouts. These abort well inside
// the budget, so a second attempt is cheap and plausibly succeeds.
//
// Note the deliberate asymmetry: a client-*Timeout* expiry always has
// elapsed ≈ budget, so it can never satisfy the gate. That is the intended
// design — a stalled upstream is never retried; a fast transport fault is.
func TestResponses_FastTransportTimeoutIsRetriedAndSucceeds(t *testing.T) {
	withResponsesBudget(t, 10*time.Second)

	// Abort any attempt whose headers don't arrive in 100ms — 1% of the
	// budget, so the retry gate opens.
	prev := copilotResponsesClient.Transport
	tr := prev.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 100 * time.Millisecond
	copilotResponsesClient.Transport = tr
	t.Cleanup(func() { copilotResponsesClient.Transport = prev })

	var attempts int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&attempts, 1) == 1 {
			// Withhold headers past ResponseHeaderTimeout.
			time.Sleep(400 * time.Millisecond)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","model":"gpt-5.6-sol","status":"completed",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`))
	}))
	defer server.Close()

	adapter := newTestAdapter(t, server.URL)
	resp, err := adapter.executeResponses(context.Background(), testRequest())

	if got := atomic.LoadInt64(&attempts); got != 2 {
		t.Fatalf("upstream called %d times, want 2 (a fast transport timeout must be retried once); err=%v", got, err)
	}
	if err != nil {
		t.Fatalf("retry should have succeeded, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response from the successful retry")
	}
}

// A caller that cancels must not trigger a retry: the retry would reuse the
// same dead context and fail instantly, adding a misleading warn log and a
// wasted upstream dial.
func TestResponses_CancelledCallerIsNotRetried(t *testing.T) {
	withResponsesBudget(t, 30*time.Second)

	var attempts int64
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		<-release
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	adapter := newTestAdapter(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if _, err := adapter.executeResponses(ctx, testRequest()); err == nil {
		t.Fatal("expected cancellation error")
	}
	if got := atomic.LoadInt64(&attempts); got != 1 {
		t.Fatalf("upstream called %d times, want exactly 1 (a cancelled caller must not be retried)", got)
	}
}

// A healthy upstream must be unaffected by the timeout split: one call, no
// retry, normal parse.
func TestResponses_HealthyUpstreamCalledOnce(t *testing.T) {
	withResponsesBudget(t, 10*time.Second)

	var attempts int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path %q, want /responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","model":"gpt-5.6-sol","status":"completed",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}],
			"usage":{"input_tokens":3,"output_tokens":1}}`))
	}))
	defer server.Close()

	adapter := newTestAdapter(t, server.URL)
	resp, err := adapter.executeResponses(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("healthy upstream returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response")
	}
	if got := atomic.LoadInt64(&attempts); got != 1 {
		t.Fatalf("upstream called %d times, want 1", got)
	}
}

// Caller cancellation must propagate immediately rather than waiting out the
// (now much larger) /responses budget.
func TestResponses_ContextCancellationBeatsBudget(t *testing.T) {
	withResponsesBudget(t, 30*time.Second)

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	adapter := newTestAdapter(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := adapter.executeResponses(ctx, testRequest())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("cancellation took %v; caller context must not be held hostage by the client budget", elapsed)
	}
}
