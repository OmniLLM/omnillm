package copilot

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// A 120s client timeout plus an unconditional retry produced the observed
// latency=240029ms before failover: the request burned its full budget, was
// retried, burned it again, and only then moved on. The retry gate exists to
// make that specific doubling impossible.
func TestWorthRetryingAfter(t *testing.T) {
	budget := 300 * time.Second

	tests := []struct {
		name    string
		elapsed time.Duration
		budget  time.Duration
		want    bool
	}{
		{name: "immediate connect failure retries", elapsed: 50 * time.Millisecond, budget: budget, want: true},
		{name: "early transport fault retries", elapsed: 10 * time.Second, budget: budget, want: true},
		{name: "just under half retries", elapsed: 149 * time.Second, budget: budget, want: true},
		{name: "exactly half does not retry", elapsed: 150 * time.Second, budget: budget, want: false},
		{name: "stalled upstream does not retry", elapsed: 299 * time.Second, budget: budget, want: false},
		{name: "full budget does not retry", elapsed: budget, budget: budget, want: false},
		{name: "overshoot does not retry", elapsed: 310 * time.Second, budget: budget, want: false},
		{name: "no budget configured retries", elapsed: time.Hour, budget: 0, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worthRetryingAfter(tt.elapsed, tt.budget); got != tt.want {
				t.Fatalf("worthRetryingAfter(%v, %v) = %v, want %v",
					tt.elapsed, tt.budget, got, tt.want)
			}
		})
	}
}

// Regression guard for the original incident: a request that consumed the
// entire budget must not be retried, capping worst-case wall-clock at one
// budget rather than two.
func TestExhaustedBudgetDoesNotDoubleWallClock(t *testing.T) {
	budget := copilotResponsesBudget()
	if budget <= 0 {
		t.Fatal("expected a positive /responses budget")
	}
	if worthRetryingAfter(budget, budget) {
		t.Fatal("a request that burned its full budget must not be retried")
	}
}

type fakeTimeoutErr struct{}

func (fakeTimeoutErr) Error() string   { return "fake timeout" }
func (fakeTimeoutErr) Timeout() bool   { return true }
func (fakeTimeoutErr) Temporary() bool { return true }

func TestShouldRetryCopilotResponsesTimeout(t *testing.T) {
	var netErr net.Error = fakeTimeoutErr{}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "wrapped deadline exceeded", err: errors.New("x: " + context.DeadlineExceeded.Error()), want: false},
		{name: "net timeout", err: netErr, want: true},
		{name: "client timeout string", err: errors.New(`Post "https://x/responses": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`), want: true},
		{name: "unrelated error", err: errors.New("connection refused"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryCopilotResponsesTimeout(tt.err); got != tt.want {
				t.Fatalf("shouldRetryCopilotResponsesTimeout(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// /responses fronts reasoning models; chat completions return in bulk. Sharing
// one budget is what made gpt-5.x calls structurally unsatisfiable.
func TestResponsesClientHasLargerBudgetThanChatClient(t *testing.T) {
	if copilotResponsesClient.Timeout <= copilotHTTPClient.Timeout {
		t.Fatalf("responses budget (%v) must exceed chat budget (%v)",
			copilotResponsesClient.Timeout, copilotHTTPClient.Timeout)
	}
	if copilotStreamClient.Timeout != 0 {
		t.Fatalf("stream client Timeout = %v, want 0", copilotStreamClient.Timeout)
	}
}
