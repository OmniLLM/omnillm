## Why

GitHub Copilot chat-completions requests fail outright when the upstream connection dies mid-flight, even though nothing has been delivered to the client yet. Runtime evidence from `~/.omnillm/omnillm.log` on 2026-08-02:

- 4 × `streaming request failed: ... Service Unavailable` (e.g. request `701b5dbe4d75b5a2`, failed after 60997ms)
- 2 × `streaming request failed: ... http2: client connection lost` (e.g. request `72c3e61ee915b54d`, failed after 45657ms)
- 1 × `stream error: stream ID 33; INTERNAL_ERROR; received from peer`
- 1 × `copilot: responses request failed: ... http2: client connection lost`

Every one of these surfaced to the caller as HTTP 502. Provider failover cannot mask them: across all 8631 dispatched requests in the log, `Trying provider for Anthropic request` appears exactly once per request, so there is no second candidate to fall through to. A single retry is the only available recovery.

These failures are distinguishable from timeouts. The connection is lost or the upstream returns 503 before any stream event reaches the client, so re-issuing the request cannot duplicate visible output.

## What Changes

- Classify a narrow set of Copilot upstream failures as transient transport failures: connection-lost/reset/unexpected-EOF transport errors, HTTP/2 `INTERNAL_ERROR` stream resets, and upstream status 502, 503, and 504.
- Retry such a failure exactly once, after a short jittered delay, for Copilot chat-completions in both streaming and non-streaming form.
- Gate the retry on no stream event having been emitted, so a mid-stream failure is never retried and output is never duplicated.
- Explicitly exclude timeouts from this retry path, preserving the existing single-attempt timeout contract.
- Emit a structured warning on each retry carrying provider, endpoint, model, attempt number, and classification reason.
- Add `request_id`, `provider`, and `upstream_model` to the terminal provider-failure error log, which currently carries only an error string and cannot be correlated with the gateway request log.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providers`: Copilot chat-completions gains a single bounded retry for pre-first-event transient transport failures.
- `routing-failover`: terminal provider-failure diagnostics become correlatable with the originating gateway request.

## Impact

- Affects `internal/providers/copilot` request execution and `internal/routes/route_logging.go` failure logging.
- Bounded blast radius: at most one duplicate upstream request, only for Copilot, only before any client-visible output.
- Does not change API shapes, streaming event serialization, authentication retry, provider failover ordering, or any timeout budget.
- Deliberately does not alter the shared `internal/providers/openaicompat` retry stub; other OpenAI-compatible providers are out of scope for this change.
