# gateway-api Specification

## Purpose
Defines OmniLLM's public client-facing HTTP contracts for OpenAI, Anthropic, and Responses API dialects, including authentication, streaming, errors, and health behavior.
## Requirements
### Requirement: Unauthenticated health endpoints
The gateway SHALL expose `/`, `/health`, and `/healthz` without authentication or concurrency accounting.

#### Scenario: Health probes
- **WHEN** a client requests `/health` or `/healthz`
- **THEN** the gateway returns a successful JSON health record or plaintext `OK`, respectively

### Requirement: OpenAI-compatible routes
The gateway SHALL serve chat completions, models, model metadata, and embeddings on both root and `/v1` mounts, while Responses SHALL be available at `/v1/responses`.

#### Scenario: Equivalent chat mounts
- **WHEN** equivalent requests are posted to `/chat/completions` and `/v1/chat/completions`
- **THEN** both are handled through the same behavior contract

### Requirement: Anthropic-compatible routes
The gateway SHALL accept Anthropic Messages requests at `/v1/messages` and SHALL estimate token counts locally at `/v1/messages/count_tokens`.

#### Scenario: Local token estimate
- **WHEN** a valid Messages payload is posted to the count endpoint
- **THEN** input tokens are estimated from prompts, content, tools, and tool results without provider dispatch

### Requirement: Consistent generation model resolution
The gateway SHALL apply the same provider-qualification, native model identifier preservation, provider pinning, provider ordering, and failover behavior to OpenAI-compatible chat completions, Anthropic Messages, and Responses generation requests.

#### Scenario: Native namespaced model across generation dialects
- **WHEN** equivalent generation requests specify a native slash-containing model identifier through chat completions, Anthropic Messages, or Responses
- **THEN** each dialect resolves and forwards the same complete native model identifier using ordinary provider selection

#### Scenario: Provider-qualified namespaced model across generation dialects
- **WHEN** equivalent generation requests specify a recognized provider qualifier followed by a slash-containing native model identifier
- **THEN** each dialect pins the same provider and forwards the same complete native model identifier after removing only the qualifier

#### Scenario: Selected provider fails
- **WHEN** an unqualified native namespaced model resolves to multiple providers and the leading provider returns a retryable failure
- **THEN** each generation dialect may continue through the same remaining provider candidates

### Requirement: Proxy authentication
When an inbound API key is configured, the gateway SHALL accept a matching bearer token, `x-api-key`, or valid stored access token and SHALL reject missing, malformed, query-only, or unknown credentials with HTTP 401.

#### Scenario: Authentication disabled
- **WHEN** the configured inbound API key is empty
- **THEN** protected proxy requests may proceed without credentials

#### Scenario: Query credential
- **WHEN** a token is provided only in the query string
- **THEN** the gateway rejects the request as unauthenticated

### Requirement: Concurrency backpressure
The gateway SHALL apply an optional post-authentication proxy concurrency limit and SHALL fail fast with HTTP 503 and `Retry-After: 1` when saturated.

#### Scenario: Limiter saturated
- **WHEN** all configured slots are occupied
- **THEN** a new proxy request receives a `server_overloaded` error without waiting

#### Scenario: Handler panic
- **WHEN** a handler panics while holding a slot
- **THEN** the slot is released before recovery completes

### Requirement: Dialect-specific streaming
The gateway SHALL stream server-sent events in the vocabulary of the requested API dialect and SHALL flush events as produced.

#### Scenario: OpenAI completion
- **WHEN** an OpenAI stream completes
- **THEN** it emits final chunks, optional usage, and literal `data: [DONE]`

#### Scenario: Anthropic completion
- **WHEN** an Anthropic stream completes
- **THEN** it emits named message and content events ending in `message_stop` without a DONE sentinel

#### Scenario: Mid-stream error
- **WHEN** upstream failure occurs after response headers are committed
- **THEN** an in-band dialect error event is emitted instead of changing HTTP status

### Requirement: Structured errors
The gateway SHALL return stable JSON error envelopes with human-readable messages, machine-readable types, and status codes reflecting client format, authentication, overload, or upstream failure.

#### Scenario: Malformed JSON
- **WHEN** request JSON is syntactically malformed
- **THEN** the gateway returns HTTP 400 with `invalid_request_error` without exposing decoder internals

#### Scenario: Upstream authentication failure
- **WHEN** all attempts end in an upstream authentication error
- **THEN** the gateway returns HTTP 401 with `authentication_error`

### Requirement: Trusted forwarding
The gateway SHALL trust forwarded client-address headers only from loopback proxies.

#### Scenario: Remote peer supplies forwarding headers
- **WHEN** a non-loopback peer supplies forwarded-address headers
- **THEN** those headers are not trusted for client identity

### Requirement: Encoding-safe log previews
Values truncated for inclusion in structured log fields SHALL be cut at a UTF-8 character boundary at or below the configured byte limit, so a truncated preview is always valid UTF-8. Truncated values SHALL retain their existing truncation marker, and values within the limit SHALL be emitted unchanged.

#### Scenario: Multi-byte character at the boundary
- **WHEN** a tool-loop preview is truncated at a byte offset that falls inside a multi-byte character
- **THEN** the preview is cut before that character and the emitted log field is valid UTF-8

#### Scenario: ASCII value truncated
- **WHEN** an ASCII value longer than the limit is truncated
- **THEN** it is cut at the limit and carries the truncation marker

#### Scenario: Value within limit
- **WHEN** a value at or below the limit is prepared for logging
- **THEN** it is emitted unchanged with no truncation marker

### Requirement: Metadata-only tool-loop diagnostics
Tool-loop diagnostic records SHALL retain non-payload metadata needed to correlate and troubleshoot a turn without emitting tool arguments or tool results. Applicable metadata SHALL include the request identifier, API shape, tool call identifier and name, relevant message, item, or block indexes, stream state, model and provider identity when known, tool error state, and payload byte length when available.

#### Scenario: Inbound raw and canonical tool result
- **WHEN** raw and canonical inbound tool-result diagnostics are emitted for the same turn
- **THEN** each record contains its applicable correlation, tool, index, and error metadata and contains neither a tool-result field nor result content

#### Scenario: Non-streaming outbound tool call
- **WHEN** a non-streaming response completes with a tool call
- **THEN** its diagnostic record contains applicable request, model, provider, tool, block, stream, and payload-length metadata and contains neither a tool-arguments field nor argument content

#### Scenario: Streaming outbound tool call
- **WHEN** streamed tool-argument deltas complete a tool call
- **THEN** the completed diagnostic record contains applicable request, model, provider, tool, block, stream, and payload-length metadata and contains neither a tool-arguments field nor reconstructed argument content

#### Scenario: Agent execution failure
- **WHEN** a client reports a failed Agent tool execution
- **THEN** the warning retains applicable request, model, tool, index, and error metadata and contains neither a tool-result field nor result content

