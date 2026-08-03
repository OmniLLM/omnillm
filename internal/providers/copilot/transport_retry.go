package copilot

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Transient-transport retry policy.
//
// A Copilot upstream attempt can die before it has produced any client-visible
// output: the HTTP/2 connection drops, the peer resets the stream, or the
// service answers 503. Those failures are recoverable -- nothing has been
// committed to the caller -- but until now they surfaced as a hard 502 because
// Copilot deployments typically have a single candidate and therefore nothing
// to fail over to.
//
// We retry such a failure exactly once. Timeouts are deliberately excluded:
// they are handled by the response-header budget and must keep their existing
// single-attempt contract.
const (
	transportRetryMinDelay = 150 * time.Millisecond
	transportRetryMaxDelay = 400 * time.Millisecond
)

// transientTransportErrorMarkers are matched against the error chain's text.
// Go does not export sentinel errors for most of these (they originate in
// golang.org/x/net/http2 and the runtime's syscall layer), so substring
// matching on a deliberately narrow set is the practical option.
var transientTransportErrorMarkers = []string{
	"client connection lost",
	"connection reset by peer",
	"connection refused",
	"broken pipe",
	"server closed idle connection",
	"http2: server sent goaway",
	"internal_error",
	"unexpected eof",
}

// transientTransportStatuses are upstream statuses that indicate the request
// never reached a model. 500 is excluded: it is frequently a deterministic
// upstream rejection of this specific payload, and retrying it just doubles
// the cost of a request that will fail again.
var transientTransportStatuses = map[int]bool{
	http.StatusBadGateway:         true,
	http.StatusServiceUnavailable: true,
	http.StatusGatewayTimeout:     true,
}

// isTransientTransportFailure reports whether a failed Copilot attempt may be
// retried once, along with a short reason suitable for structured logging.
//
// Pass statusCode == 0 when classifying a transport-level error with no HTTP
// response, and err == nil when classifying a non-2xx response.
//
// Timeouts and context cancellation are checked first and always return false,
// so a failure that looks like both a timeout and a connection error can never
// leak into the retry path.
func isTransientTransportFailure(err error, statusCode int) (bool, string) {
	if err != nil {
		// Order matters: timeout and cancellation win over every marker below.
		if isTimeoutError(err) {
			return false, ""
		}
		if errors.Is(err, context.Canceled) {
			return false, ""
		}

		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return true, "unexpected_eof"
		}

		msg := strings.ToLower(err.Error())
		for _, marker := range transientTransportErrorMarkers {
			if strings.Contains(msg, marker) {
				return true, normalizeTransportReason(marker)
			}
		}
		return false, ""
	}

	if transientTransportStatuses[statusCode] {
		return true, "upstream_status"
	}
	return false, ""
}

// normalizeTransportReason turns a matched marker into a stable log token.
func normalizeTransportReason(marker string) string {
	return strings.ReplaceAll(strings.TrimSpace(marker), " ", "_")
}

// transportRetryDelay returns a randomized backoff. Jitter is not cosmetic
// here: a Copilot-side 503 tends to hit many in-flight requests at once, and a
// fixed delay would resynchronize them into a second simultaneous burst
// against an already-struggling upstream.
func transportRetryDelay() time.Duration {
	spread := transportRetryMaxDelay - transportRetryMinDelay
	if spread <= 0 {
		return transportRetryMinDelay
	}
	return transportRetryMinDelay + time.Duration(rand.Int63n(int64(spread)))
}

// waitBeforeTransportRetry sleeps for a jittered interval, returning false if
// the context is canceled first so a disconnected client does not pay for the
// wait.
func waitBeforeTransportRetry(ctx context.Context) bool {
	timer := time.NewTimer(transportRetryDelay())
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
