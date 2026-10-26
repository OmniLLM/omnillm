## Why

The supplied Codex Responses log excerpt contains five `Failed to read request
body` errors with `unexpected EOF`. Four have completed HTTP records lasting
1.115–1.263 seconds and are followed by successful new requests. These failures
occur before request parsing or provider dispatch. Go's HTTP body reader reports
unexpected EOF when an upload ends before its declared length or chunk framing
is complete. The excerpt cannot identify the component closing the connection
or prove that subsequent requests are retries of identical payloads.

The current implementation has two confirmed diagnostic defects:

- Generation handlers log every body-read failure at ERROR, including incomplete
  uploads and size-limit violations, without API, client, or byte-count context.
  The shared reader discards the partial byte count on failure.
- The server log-stream formatter suppresses `method`, `path`, and `status`, so
  CLI and console subscribers see HTTP records without the request outcome.

The server has no body `ReadTimeout`; its 15-second header timeout and 120-second
idle timeout do not explain the observed body-read durations. The existing
16 MiB body limit produces a different error. Successful `tool_use` responses
and increasing history sizes are not by themselves evidence of a tool-loop bug.

## What Changes

- Preserve failure byte counts as metadata while continuing to discard partial
  request payloads and stop before parsing or dispatch.
- Share body-read diagnostic classification across Chat Completions, Responses,
  Messages, token counting, and System One, all existing bounded-reader callers.
- Report incomplete uploads and oversize requests at WARN, explicit request
  cancellation at INFO, and otherwise unclassified read failures at ERROR.
- Include safe request identity, API, method, query-free path, client, bounded
  user agent, bytes received, declared content length, and context state.
- Retain existing HTTP 400/413 response mappings and endpoint error envelopes.
- Preserve HTTP method, query-free path, and status in the formatted log stream.
- Add deterministic failure-path, privacy, formatter, and client compatibility
  coverage before implementation, after approval.

This change affects runtime diagnostics. It does not claim to repair a client
or intermediary transport interruption, increase timeouts or body limits, retry
partial uploads, or change provider execution.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-api`: metadata-only body-read failure diagnostics and query-free HTTP
  access-log paths.
- `cli-ops-config`: HTTP outcome fields retained by server log-tail formatting.

## Impact

Expected implementation areas are `internal/routes/request_body.go`, its five
handler callers, `internal/server/server.go`, `internal/server/logstream.go`, and
associated route, server, and CLI tests. Existing console parsing already keeps
fields supplied by the server and should require no UI implementation change.

No implementation is included in this proposal. Human approval remains pending.
