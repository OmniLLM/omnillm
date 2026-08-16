## ADDED Requirements

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
