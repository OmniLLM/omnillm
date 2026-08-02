## Purpose

Defines OmniLLM's public client-facing HTTP contracts for OpenAI, Anthropic, and Responses API dialects, including authentication, streaming, errors, and health behavior.

## ADDED Requirements

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
