package shared

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

// Default upstream timeouts.
//
// DefaultRequestTimeout suits non-streaming chat completions, which return in
// bulk. DefaultResponsesTimeout is larger because the /responses endpoint
// fronts reasoning models (gpt-5.x and friends) that can think for minutes
// before emitting response headers — a 120s cap makes those requests
// structurally impossible to satisfy rather than merely slow.
const (
	DefaultRequestTimeout   = 120 * time.Second
	DefaultResponsesTimeout = 300 * time.Second
)

// TimeoutFromEnv reads a duration override from the named environment
// variable, falling back to def when unset or unparseable. Accepts either a Go
// duration ("240s", "5m") or a bare number of seconds ("240"), so operators can
// tune timeouts without a rebuild.
func TimeoutFromEnv(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return def
}

// DefaultHTTPTransport returns a new *http.Transport with production-safe
// connection pool settings. Every provider client should use this instead of
// repeating the same boilerplate or relying on Go's default Transport
// (which has MaxIdleConnsPerHost=2 — far too low under concurrent load).
//
// Callers that need a streaming client should omit Timeout (set it to 0) so
// long-lived SSE connections are not cut off by an idle timeout.
func DefaultHTTPTransport() *http.Transport {
	t := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Reasoning models can think for minutes before emitting their first SSE
	// token. Without HTTP/2 keepalive pings, an idle-but-healthy stream is
	// indistinguishable from a dead one to intermediaries, which eventually
	// reset it — surfacing as "stream error: ...; INTERNAL_ERROR; received
	// from peer" after a long silence. Pinging keeps the connection provably
	// alive and, just as importantly, fails fast when it genuinely is dead
	// instead of hanging until the client gives up.
	if h2, err := http2.ConfigureTransports(t); err == nil {
		h2.ReadIdleTimeout = 30 * time.Second
		h2.PingTimeout = 15 * time.Second
	}

	return t
}

// DefaultHTTPClient returns an *http.Client suitable for non-streaming
// (request-response) provider calls. Uses DefaultHTTPTransport.
func DefaultHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: DefaultHTTPTransport(),
	}
}

// DefaultResponseHeaderClient returns an *http.Client whose timeout applies
// only while waiting for upstream response headers. Response bodies remain
// governed by caller cancellation and transport health, which is required for
// long-lived SSE streams.
func DefaultResponseHeaderClient(timeout time.Duration) *http.Client {
	return responseHeaderClient(timeout, 0)
}

// DefaultBoundedResponseHeaderClient also bounds the complete response. It is
// suitable for non-streaming reasoning calls that need a larger header budget
// but must not hang indefinitely after headers arrive.
func DefaultBoundedResponseHeaderClient(headerTimeout, totalTimeout time.Duration) *http.Client {
	return responseHeaderClient(headerTimeout, totalTimeout)
}

func responseHeaderClient(headerTimeout, totalTimeout time.Duration) *http.Client {
	transport := DefaultHTTPTransport()
	transport.ResponseHeaderTimeout = headerTimeout
	return &http.Client{Timeout: totalTimeout, Transport: transport}
}

// DefaultStreamClient returns an *http.Client suitable for streaming (SSE)
// provider calls — no Timeout so long-lived connections are not cut off.
func DefaultStreamClient() *http.Client {
	return &http.Client{
		Transport: DefaultHTTPTransport(),
	}
}
