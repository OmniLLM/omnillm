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

### Requirement: Bounded inbound request bodies
The gateway SHALL enforce a finite request-body limit for Chat Completions, Responses, Messages, and Messages token-counting requests and SHALL return HTTP 413 with the dialect's structured invalid-request error when the limit is exceeded.

#### Scenario: Generation request exceeds the limit
- **WHEN** a client posts a generation request body larger than the configured gateway limit
- **THEN** the gateway returns HTTP 413 without parsing or dispatching the request

#### Scenario: Token-count request exceeds the limit
- **WHEN** a client posts a Messages token-counting body larger than the configured gateway limit
- **THEN** the gateway returns HTTP 413 without attempting local token estimation

#### Scenario: Request remains within the limit
- **WHEN** a syntactically valid request body is at or below the configured gateway limit
- **THEN** existing parsing, routing, and response behavior remains unchanged

### Requirement: Cancellable rate-limit waiting
A gateway request waiting for an available rate-limit time SHALL observe its request context and SHALL stop waiting promptly when the client cancels or the server cancels the request.

#### Scenario: Client cancels queued request
- **WHEN** a request context is canceled while waiting for a rate-limit reservation
- **THEN** the wait ends promptly and the canceled reservation does not delay later active requests

#### Scenario: Active queued request reaches reservation
- **WHEN** a queued request remains active until its reserved time
- **THEN** it proceeds under the configured request-spacing contract

### Requirement: OpenAI tool-call finish reason
An OpenAI-compatible non-streaming response containing one or more assistant tool calls SHALL report `finish_reason` as `tool_calls`, even when upstream terminal metadata reports an ordinary stop.

#### Scenario: Canonical tool use has inconsistent upstream stop
- **WHEN** a provider response contains canonical tool calls but carries an upstream end-turn or stop reason
- **THEN** the OpenAI-compatible choice reports `finish_reason: tool_calls` and preserves the tool calls for replay

#### Scenario: Ordinary terminal response
- **WHEN** a provider response contains no tool calls and terminates normally
- **THEN** the OpenAI-compatible choice retains the ordinary `stop` finish reason

### Requirement: Responses custom-tool history input
The `/v1/responses` endpoint SHALL accept valid, explicitly typed `custom_tool_call` and `custom_tool_call_output` history items and SHALL process them through ordinary provider dispatch after canonical normalization.

#### Scenario: Droid custom-tool continuation
- **WHEN** Droid submits a Responses history containing a valid custom tool call and its associated output
- **THEN** the gateway accepts the request and reaches provider dispatch instead of returning an unknown-item HTTP 400

#### Scenario: Missing custom call identifier
- **WHEN** a custom tool call or output omits `call_id`
- **THEN** the gateway returns HTTP 400 with a structured invalid-request error

#### Scenario: Missing custom call name
- **WHEN** a custom tool call omits its name
- **THEN** the gateway returns HTTP 400 with a structured invalid-request error

#### Scenario: Missing versus empty custom input
- **WHEN** a custom tool call omits `input`
- **THEN** the gateway returns HTTP 400, while an explicitly present empty string is accepted

#### Scenario: Missing versus empty custom output
- **WHEN** a custom tool output omits `output`
- **THEN** the gateway returns HTTP 400, while an explicitly present empty string or supported empty content list is accepted

#### Scenario: Invalid custom output shape
- **WHEN** a custom tool output is neither a string nor a list of supported output content items
- **THEN** the gateway returns HTTP 400 with a structured invalid-request error

### Requirement: Native Responses custom-tool output
The public Responses API SHALL serialize canonical custom tool calls as native `custom_tool_call` items with their original declared name and raw input.

#### Scenario: Non-streaming custom call
- **WHEN** a canonical response contains a custom tool call
- **THEN** the Responses output contains `type: custom_tool_call`, the original `call_id`, name, raw `input`, and no function `arguments`

#### Scenario: Streaming custom input
- **WHEN** a provider streams a custom tool call and raw input
- **THEN** the Responses stream emits a custom output-item announcement followed by `response.custom_tool_call_input.delta`, `response.custom_tool_call_input.done`, output-item completion, and final response completion in order

#### Scenario: Function call remains unchanged
- **WHEN** a canonical response contains an ordinary function call
- **THEN** Responses serialization continues to emit existing `function_call` items and function-argument events

### Requirement: Anthropic prompt-cache input
The Anthropic Messages route SHALL accept `cache_control` with `type: ephemeral` and optional TTL `5m` or `1h` at request level, on structured system blocks, on supported message content blocks, and on tool definitions.

#### Scenario: Explicit cache breakpoints
- **WHEN** a Messages request marks system, message, or tool content using valid cache controls
- **THEN** the request reaches provider dispatch with the same controls attached to the same canonical elements

#### Scenario: Automatic cache control
- **WHEN** a Messages request supplies a valid top-level cache control
- **THEN** canonical request policy records automatic caching without OmniLLM inventing explicit breakpoint placement

### Requirement: Prompt-cache request validation
The gateway SHALL reject an unsupported cache-control type, unsupported TTL, malformed cache-control value, unsupported placement, or more than four effective explicit breakpoints with HTTP 400 in the caller's structured error dialect.

#### Scenario: Fifth explicit breakpoint
- **WHEN** a request contains five explicit cache-control breakpoints across system, messages, and tools
- **THEN** provider dispatch does not occur and the client receives a structured invalid-request error

#### Scenario: Invalid TTL
- **WHEN** a caller supplies an Anthropic cache-control TTL other than `5m` or `1h`
- **THEN** provider dispatch does not occur and the client receives a structured invalid-request error

### Requirement: OpenAI-native prompt-cache input
The Chat Completions and Responses routes SHALL accept optional `prompt_cache_key` and `prompt_cache_retention` fields and SHALL preserve omission separately from supplied values.

#### Scenario: Native cache hints
- **WHEN** a Chat Completions or Responses request supplies native cache key and retention values
- **THEN** provider dispatch receives those values as native controls rather than Anthropic breakpoints

### Requirement: Cache usage output parity
Non-streaming and streaming responses SHALL expose provider-reported cache-read and cache-write usage using the standard vocabulary of the requested API dialect without changing aggregate token totals.

#### Scenario: Anthropic non-streaming cache usage
- **WHEN** canonical usage includes cache-read and cache-write input and the caller requested Messages
- **THEN** the response reports `input_tokens` as uncached input and includes `cache_read_input_tokens` and `cache_creation_input_tokens`

#### Scenario: OpenAI stream cache usage
- **WHEN** canonical stream-final usage includes cache-read input and the caller requested Chat Completions
- **THEN** the final usage chunk includes the total prompt tokens and `prompt_tokens_details.cached_tokens`

#### Scenario: Responses cache usage
- **WHEN** canonical usage includes cache-read input and the caller requested Responses
- **THEN** usage includes total `input_tokens` and `input_tokens_details.cached_tokens`

### Requirement: Prompt-cache logging privacy
Prompt-cache diagnostics SHALL contain only request, provider, model, mode, placement-count, and usage metadata and SHALL NOT include prompt content, tool arguments, tool results, credentials, raw cache keys, or cacheable-prefix bytes.

#### Scenario: Unsupported directive omitted
- **WHEN** an adapter records that it omitted unsupported cache metadata
- **THEN** the diagnostic identifies the provider policy and marker count without including request payload content

### Requirement: Responses function-tool history input
The `/v1/responses` endpoint SHALL accept an ordinary `function_call_output` whose explicitly present `output` is either a string or an ordered list containing only `input_text`, `input_image`, or `input_file` content items valid under the maintained OpenAI Responses schema, SHALL preserve the associated `call_id`, and SHALL process the normalized history through ordinary provider dispatch. The endpoint SHALL reject missing outputs, unsupported output value kinds, malformed content members, unsupported member types, and invalid values for fields that are present with HTTP 400 and a structured `invalid_request_error` before provider dispatch.

#### Scenario: String function output continuation
- **WHEN** a client submits a Responses history containing a function call and a string output with the matching `call_id`
- **THEN** the gateway accepts the request and reaches provider dispatch with the call/result association preserved

#### Scenario: Structured function output continuation
- **WHEN** a client submits a Responses history containing a function call and an ordered valid text, image, or file output list with the matching `call_id`
- **THEN** the gateway accepts the request, preserves the complete ordered output, and reaches provider dispatch

#### Scenario: Explicit empty function output
- **WHEN** a function output is an explicitly present empty string or empty content list
- **THEN** the gateway accepts it and distinguishes it from an omitted output

#### Scenario: Missing function output
- **WHEN** a `function_call_output` omits `output`
- **THEN** the gateway returns HTTP 400 with a structured `invalid_request_error` before provider dispatch

#### Scenario: Invalid function output value
- **WHEN** a `function_call_output.output` is an object, scalar other than string, null, or a list containing a non-object member
- **THEN** the gateway returns HTTP 400 with a structured `invalid_request_error` before provider dispatch

#### Scenario: Type-only and nullable image or file content
- **WHEN** a function output list contains an `input_image` or `input_file` member with only its required type, or with optional fields explicitly set to null
- **THEN** the gateway accepts the schema-valid member, preserves it unchanged, and reaches provider dispatch

#### Scenario: Invalid function output content item
- **WHEN** a function output list contains an unsupported type, an `input_text` member without string text, or an optional image/file field with an invalid supplied type or enum value
- **THEN** the gateway returns HTTP 400 with a structured `invalid_request_error` before provider dispatch

### Requirement: Responses exact-cache behavior
The `/v1/responses` endpoint SHALL apply the operator-enabled exact-response cache to non-streaming and streaming requests, SHALL preserve existing per-request bypass and disable controls, and SHALL execute the provider normally on a miss or incompatible cached value.

#### Scenario: Non-streaming Responses miss and hit
- **WHEN** two semantically identical non-streaming Responses requests execute while caching is enabled
- **THEN** the first successful provider result is marked as a miss and stored, and the second is marked as a hit and returned without another provider execution

#### Scenario: Streaming Responses miss and hit
- **WHEN** a complete Responses stream populates the cache and an equivalent streaming request follows
- **THEN** the second request emits valid Responses SSE events from the canonical entry with exactly one `response.completed` event

#### Scenario: Responses cache bypass
- **WHEN** a Responses request uses the existing bypass or refresh control
- **THEN** the cache read is skipped, ordinary provider execution occurs, and the successful complete result may refresh the entry

#### Scenario: Responses cache disabled per request
- **WHEN** a Responses request uses the existing off or disable control
- **THEN** neither a cache read nor a cache write occurs

#### Scenario: Incompatible cached value
- **WHEN** a cached canonical value cannot be converted into a valid Responses object or stream
- **THEN** the gateway does not commit a hit response and continues through ordinary provider execution

### Requirement: Cached Responses native-tool fidelity
Cached Responses replay SHALL preserve ordinary function calls and native custom tool calls in their respective public event and object vocabularies, including identifiers, declared names, raw input, optional namespace, ordering, and call-result relationships.

#### Scenario: Cached custom tool response
- **WHEN** a cached canonical response contains a native custom tool call
- **THEN** non-streaming Responses output contains `custom_tool_call` with the original call identifier, name, raw input, and namespace when present

#### Scenario: Cached custom tool stream
- **WHEN** a streaming Responses request hits a cached native custom tool call
- **THEN** the stream emits custom input delta and done events in order and does not emit function-argument events for that custom call

#### Scenario: Cached function tool remains unchanged
- **WHEN** a cached canonical response contains an ordinary function call
- **THEN** Responses replay continues emitting `function_call` items and function-argument events
