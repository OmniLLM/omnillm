## Context and evidence

`readGatewayRequestBody` bounds input with `io.LimitReader` and returns nil on
read error. Its callers correctly stop, but lose the partial byte count.
Chat Completions, Responses, and Messages log every such error at ERROR; token
counting and System One use the reader without an equivalent diagnostic.
`gatewayRequestBodyError` already maps oversize input to 413 and other failures
to 400. These response contracts will remain intact.

`buildRouter` records method, RequestURI, and status in its HTTP middleware.
`formatStructuredField` in the server log stream explicitly discards all three.
The CLI merely renders the remaining fields; the console also accepts arbitrary
supplied fields. Restoring fields at the server formatter fixes both consumers.
Before exposing path there, replace RequestURI with URL.Path so query values do
not become visible in formatted logs or remain in raw access logs.

Existing focused body-limit, reader-error, route, and broadcast formatter tests
passed during investigation. They do not cover interrupted uploads or retention
of HTTP outcome fields.

## Decisions

1. Keep `readGatewayRequestBody`'s payload/error contract: failure returns no
   usable body. Carry received-byte metadata in a small wrapping error with
   `Unwrap` so `errors.Is` and existing size-limit status mapping still work.
   Count bytes from the same read that returns the error. Bounded reads consume
   at most the existing limit plus one; logged byte counts describe actual
   consumed bytes, not an invented total size.
2. Use one diagnostic helper across all five callers. Keep response serialization
   with the handlers so System One retains `systemone_error` and other routes
   retain their existing `invalid_request_error` envelopes.
3. Classify using `errors.Is`, not string matching. Oversize and unexpected EOF
   are WARN. Explicit context cancellation is INFO only when both the read error
   and request context establish cancellation. Other errors remain ERROR even
   if context cancellation happens concurrently. Record context state separately
   so EOF and cancellation can both be observed without guessing the initiator.
4. Emit normalized reason and context fields, not arbitrary error text. Preserve
   the original error chain internally. This keeps metadata useful without
   accidentally logging synthetic or real payload data from a custom reader.
5. Use the existing trusted `ClientIP` behavior and a bounded, encoding-safe
   user-agent field. Do not extract a model from an incomplete body. Do not log
   raw headers, bodies, query strings, tool contents, or credentials.
6. Retain method, path, and status in the shared server formatter's ordered
   fields. Keep existing response-summary aliases and zero-token suppression.
   Verify downstream CLI and console field preservation; change those consumers
   only if a regression test demonstrates a need.

## Compatibility and failure handling

No route shape, status mapping, body-size limit, provider selection, retry policy,
or SSE behavior changes. A disconnected client may never receive the attempted
400 response; a logged status is the handler's selected outcome, not proof of
delivery. Incomplete uploads must never be salvaged or retried inside OmniLLM.

The shared helper also serves System One, so its error-envelope compatibility
requires deterministic coverage despite the report originating from Responses.
The coding-agent ingestion policy requires existing five-turn deterministic
coverage for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI, plus bounded
live client checks where prerequisites are available. Live checks use an isolated
fresh-port gateway and record explicit skip reasons when unavailable.

## Verification

- Reader fixtures: partial bytes plus wrapped unexpected EOF, explicit canceled
  context, unrelated failure with canceled context, unknown length, oversize,
  exact boundary, and successful reads.
- Handler matrix: all five callers preserve status/envelope, severity, byte
  evidence, request metadata, and no dispatch after failed reads.
- A real loopback HTTP test with a short declared-length upload and a client
  write-half-close confirms the net/http EOF path, without contacting providers.
  Exercise incomplete chunked framing as well if supported by that fixture.
- Privacy sentinels in partial body, reader error, authorization, and query;
  UTF-8 user-agent truncation and wrapped-error matching.
- HTTP middleware/formatter/CLI and console parsing checks retain method/path/
  status and exclude query values without regressing response summaries.
- Required spec, Go, Bun, and bounded client compatibility checks in tasks.md.

## Rollout and remaining uncertainty

Deploy through the normal operator workflow after validation and approval of the
implementation. No configuration migration is needed. Future failure records
will let an operator compare received and advertised bytes and correlate the
request with client or proxy logs. If interruptions persist, those external logs
are needed to identify the connection closer; merely changing log severity does
not repair the underlying transport. No timeout or provider changes are justified
by the supplied evidence.
