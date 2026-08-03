# Design: Copilot transient transport retry

## Context

Copilot chat-completions currently issues exactly one upstream attempt per candidate. `executeOpenAIStreamWithRetry` (`internal/providers/copilot/adapter.go`) performs a bare `client.Do(req)` and, on transport error, wraps it as `streaming request failed: %w` and returns. The only existing retry paths are semantic, not transport-level: an auth-error retry after token refresh, and a fallback to the `/responses` API for models that reject chat-completions.

The shared `internal/providers/openaicompat/http.go` has a retry extension point, but it is inert: `clonePOSTRetryRequest` returns nil for any streaming request, and `shouldRetryPOSTTimeout` is hardcoded `return false`. This change deliberately leaves that stub alone and implements the retry inside the Copilot adapter, keeping blast radius to the one provider where failures were observed.

## Goals

- Recover from pre-first-event transient upstream failures without surfacing a 502.
- Guarantee no duplicated client-visible output under any interleaving.
- Preserve the existing single-attempt timeout contract established by `2026-08-02-harden-copilot-claude-timeout`.

## Non-goals

- Retrying other provider types or the shared openaicompat layer.
- Multi-attempt or exponential backoff policies.
- Changing timeout budgets, failover ordering, or any API shape.

## Decisions

### Classification lives in one predicate

A single `isTransientTransportFailure(err error, statusCode int) (bool, string)` helper in the copilot package returns both the decision and a short human-readable reason for the log line. Centralizing it keeps the streaming and non-streaming call sites honest and gives tests one target.

Classified transient:
- transport errors whose chain matches connection-lost, connection-reset, or `io.ErrUnexpectedEOF`
- HTTP/2 `INTERNAL_ERROR` stream resets
- upstream status 502, 503, 504

Explicitly NOT transient:
- anything satisfying the existing `isTimeoutError`, checked *first* so a timeout that also looks like a connection error cannot leak into the retry path
- context cancellation from client disconnect
- all 4xx, including 400 and auth failures

### Retry only before the first emitted event

This is the correctness crux. The retry decision is made only at the point where we still hold the raw `*http.Response` and have not yet spawned `parseOpenAISSE`. Once the SSE goroutine starts, a mid-stream failure is reported through the event channel and is structurally outside the retry path. That means "no event emitted" is enforced by control-flow position rather than by a mutable flag, which removes the race entirely.

Rejected alternative: tracking an `emitted bool` inside the SSE parser and retrying from there. That would require buffering or replaying partial output and could duplicate tokens on any misjudgment. Not worth it for the recovery gained.

### Jitter

A single retry after a randomized delay in the 150–400 ms range. Randomization matters because a Copilot-side 503 tends to hit many in-flight requests at once; a fixed delay would resynchronize them into a second simultaneous burst against an already-struggling upstream.

The delay respects context cancellation, so a client that disconnects during the backoff does not pay for the wait.

### Retry counts as the same candidate

The retry happens inside the adapter, below `providerdispatch`. Dispatch sees one logical attempt with one final error, so failover ordering and `AbortError` semantics are untouched. If both attempts fail, the second attempt's error propagates — it is the more recent evidence about upstream state.

## Risks

- **Duplicate upstream billing on retry.** Bounded at one extra request, and only when the first produced no usable response. Accepted.
- **Retrying a 503 that is really a sustained outage.** Bounded by the single-retry cap; the second failure propagates normally to dispatch.
- **Misclassifying a non-idempotent failure.** Mitigated by requiring zero emitted events, which means the upstream produced no committed output we could be duplicating.

## Migration

None. No config, schema, or API surface changes. The behavior is strictly additive on a path that previously always failed.
