package copilot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"omnillm/internal/cif"
)

// --- Task 1.3: classifier ---------------------------------------------------

func TestIsTransientTransportFailure_Classification(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		want       bool
		wantReason string
	}{
		// Transient transport errors.
		{"http2 connection lost", errors.New(`Post "https://x/chat/completions": http2: client connection lost`), 0, true, "client_connection_lost"},
		{"connection reset", errors.New("read tcp 1.2.3.4:443: connection reset by peer"), 0, true, "connection_reset_by_peer"},
		{"http2 internal error", errors.New("stream error: stream ID 33; INTERNAL_ERROR; received from peer"), 0, true, "internal_error"},
		{"unexpected eof sentinel", fmt.Errorf("reading body: %w", io.ErrUnexpectedEOF), 0, true, "unexpected_eof"},
		{"broken pipe", errors.New("write tcp: broken pipe"), 0, true, "broken_pipe"},

		// Transient upstream statuses.
		{"status 502", nil, http.StatusBadGateway, true, "upstream_status"},
		{"status 503", nil, http.StatusServiceUnavailable, true, "upstream_status"},
		{"status 504", nil, http.StatusGatewayTimeout, true, "upstream_status"},

		// Excluded: timeouts must keep their single-attempt contract.
		{"deadline exceeded", context.DeadlineExceeded, 0, false, ""},
		{"client timeout", errors.New(`net/http: request canceled (Client.Timeout exceeded while awaiting headers)`), 0, false, ""},

		// Excluded: cancellation and deterministic rejections.
		{"context canceled", context.Canceled, 0, false, ""},
		{"status 400", nil, http.StatusBadRequest, false, ""},
		{"status 401", nil, http.StatusUnauthorized, false, ""},
		{"status 500", nil, http.StatusInternalServerError, false, ""},
		{"status 200", nil, http.StatusOK, false, ""},
		{"unrelated error", errors.New("failed to marshal request"), 0, false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := isTransientTransportFailure(tc.err, tc.statusCode)
			if got != tc.want {
				t.Fatalf("isTransientTransportFailure(%v, %d) = %v, want %v", tc.err, tc.statusCode, got, tc.want)
			}
			if reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

// A failure that reads as BOTH a timeout and a connection error must be
// classified as a timeout, never retried. This is the ordering guarantee the
// design depends on.
func TestIsTransientTransportFailure_TimeoutWinsOverMarker(t *testing.T) {
	err := fmt.Errorf("client.Timeout exceeded while awaiting headers: http2: client connection lost")
	if transient, _ := isTransientTransportFailure(err, 0); transient {
		t.Fatal("timeout-shaped error was classified transient; it must not be retried")
	}
}

// --- Task 2.3: backoff ------------------------------------------------------

func TestTransportRetryDelay_WithinBounds(t *testing.T) {
	for i := 0; i < 200; i++ {
		d := transportRetryDelay()
		if d < transportRetryMinDelay || d >= transportRetryMaxDelay {
			t.Fatalf("delay %v outside [%v, %v)", d, transportRetryMinDelay, transportRetryMaxDelay)
		}
	}
}

func TestWaitBeforeTransportRetry_AbortsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if waitBeforeTransportRetry(ctx) {
		t.Fatal("wait returned true on a canceled context")
	}
	if elapsed := time.Since(start); elapsed > transportRetryMinDelay {
		t.Fatalf("wait took %v; should return promptly on cancellation", elapsed)
	}
}

// --- Task 6: end-to-end retry behavior --------------------------------------

const sseChunk = "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"index\":0}]}\n\n" +
	"data: [DONE]\n\n"

func writeSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, sseChunk)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeChatJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`)
}

func drain(t *testing.T, ch <-chan cif.CIFStreamEvent) int {
	t.Helper()
	count := 0
	for range ch {
		count++
	}
	return count
}

// 6.1: connection dropped on attempt 1, success on attempt 2.
func TestStream_ConnectionDroppedIsRetriedOnce(t *testing.T) {
	var attempts int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&attempts, 1) == 1 {
			// Hijack and slam the socket shut without a response.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack failed: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		writeSSE(w)
	}))
	defer server.Close()

	adapter := newTestAdapter(t, server.URL)
	ch, err := adapter.executeOpenAIStream(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("expected recovery via retry, got error: %v", err)
	}
	if got := drain(t, ch); got == 0 {
		t.Fatal("retry produced no stream events")
	}
	if got := atomic.LoadInt64(&attempts); got != 2 {
		t.Fatalf("upstream attempts = %d, want exactly 2", got)
	}
}

// 6.2: 503 then 200, for both streaming and non-streaming.
func TestUpstream503IsRetriedOnce(t *testing.T) {
	for _, stream := range []bool{true, false} {
		name := "nonstreaming"
		if stream {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			var attempts int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if atomic.AddInt64(&attempts, 1) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"error":{"message":"unavailable"}}`)
					return
				}
				if stream {
					writeSSE(w)
					return
				}
				writeChatJSON(w)
			}))
			defer server.Close()

			adapter := newTestAdapter(t, server.URL)
			var err error
			if stream {
				var ch <-chan cif.CIFStreamEvent
				ch, err = adapter.executeOpenAIStream(context.Background(), testRequest())
				if err == nil {
					drain(t, ch)
				}
			} else {
				_, err = adapter.executeOpenAI(context.Background(), testRequest())
			}
			if err != nil {
				t.Fatalf("expected recovery after 503, got error: %v", err)
			}
			if got := atomic.LoadInt64(&attempts); got != 2 {
				t.Fatalf("upstream attempts = %d, want exactly 2", got)
			}
		})
	}
}

// 6.3: a failure AFTER headers/events must not be retried. The upstream sends
// a valid SSE header, emits one event, then dies mid-body.
func TestStream_MidStreamFailureIsNotRetried(t *testing.T) {
	var attempts int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"index\":0}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()

	adapter := newTestAdapter(t, server.URL)
	ch, err := adapter.executeOpenAIStream(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("stream setup should succeed: %v", err)
	}
	drain(t, ch)

	if got := atomic.LoadInt64(&attempts); got != 1 {
		t.Fatalf("upstream attempts = %d, want exactly 1: output was already delivered and must never be replayed", got)
	}
}

// 6.4: timeouts keep their single-attempt contract.
func TestStream_TimeoutIsNotRetried(t *testing.T) {
	const budget = 200 * time.Millisecond
	prev := copilotChatStreamClient.Timeout
	copilotChatStreamClient.Timeout = budget
	t.Cleanup(func() { copilotChatStreamClient.Timeout = prev })

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
	if _, err := adapter.executeOpenAIStream(context.Background(), testRequest()); err == nil {
		t.Fatal("expected timeout error")
	}
	if got := atomic.LoadInt64(&attempts); got != 1 {
		t.Fatalf("upstream attempts = %d, want exactly 1 (timeouts must not be retried)", got)
	}
}

// 6.5: both attempts fail -> final error propagates, no third attempt.
func TestStream_BothAttemptsFailStopsAtTwo(t *testing.T) {
	var attempts int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"still unavailable"}}`)
	}))
	defer server.Close()

	adapter := newTestAdapter(t, server.URL)
	_, err := adapter.executeOpenAIStream(context.Background(), testRequest())
	if err == nil {
		t.Fatal("expected error when both attempts fail")
	}
	if got := atomic.LoadInt64(&attempts); got != 2 {
		t.Fatalf("upstream attempts = %d, want exactly 2", got)
	}
}

// Non-transient failures must not gain an extra attempt.
func TestStream_BadRequestIsNotRetried(t *testing.T) {
	var attempts int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"bad","code":"invalid_request_error"}}`)
	}))
	defer server.Close()

	adapter := newTestAdapter(t, server.URL)
	if _, err := adapter.executeOpenAIStream(context.Background(), testRequest()); err == nil {
		t.Fatal("expected error on 400")
	}
	if got := atomic.LoadInt64(&attempts); got != 1 {
		t.Fatalf("upstream attempts = %d, want exactly 1", got)
	}
}
