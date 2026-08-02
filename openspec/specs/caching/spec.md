# caching Specification

## Purpose
Defines the opt-in exact-match canonical response cache, including deterministic eligibility, semantic keys, request controls, replay, persistence, expiry, and administration.
## Requirements
### Requirement: Opt-in live configuration
The response cache SHALL remain disabled unless explicitly enabled and SHALL read enabled state and positive TTL per request, defaulting TTL to one hour.

#### Scenario: Configuration changes
- **WHEN** an operator changes cache enablement or TTL
- **THEN** subsequent requests observe the change without restart

### Requirement: Deterministic eligibility
Only requests with explicit temperature zero and top-p unset or at least one SHALL be cache eligible; streaming intent SHALL not make an otherwise deterministic request ineligible.

#### Scenario: Temperature omitted
- **WHEN** a request omits temperature
- **THEN** it is not cached because provider defaults cannot be assumed deterministic

### Requirement: Semantic cache key
The key SHALL be stable over generation-affecting canonical fields and SHALL exclude transport headers, user metadata, and streaming mode.

#### Scenario: Cross-mode key
- **WHEN** otherwise identical streaming and non-streaming requests are keyed
- **THEN** they produce the same key

### Requirement: Route scope
Caching SHALL apply to chat completions and Anthropic messages and SHALL not apply to Responses or embeddings.

#### Scenario: Responses request
- **WHEN** an eligible request uses `/v1/responses`
- **THEN** no response-cache read or write occurs

### Requirement: Per-request controls
The cache-control header SHALL support bypass or refresh for skipping reads while allowing writes and off or disable for skipping both.

#### Scenario: Refresh requested
- **WHEN** a client requests refresh
- **THEN** the gateway calls upstream and stores the fresh successful result

### Requirement: Canonical replay
A hit SHALL be re-serialized into the current caller's dialect and streaming mode and labeled `X-OmniLLM-Cache: hit`.

#### Scenario: Streaming cache hit
- **WHEN** a streaming request hits an entry
- **THEN** the canonical response is synthesized into normal dialect stream events

#### Scenario: Serialization fails
- **WHEN** a stored response cannot be serialized for the caller
- **THEN** the gateway warns and falls through to upstream execution

### Requirement: Best-effort population
Cache writes SHALL never fail a successful request and SHALL exclude errored, incomplete, empty, or unsupported canonical responses.

#### Scenario: Incomplete stream
- **WHEN** a stream errors or ends without a terminal event
- **THEN** no entry is stored

### Requirement: Entry lifecycle
Entries SHALL expire by creation age, increment hit accounting best-effort, be replaceable by key, and support bulk expiry purge and complete clear.

#### Scenario: Expired read
- **WHEN** an entry is older than the active TTL
- **THEN** lookup returns a miss and leaves the row for later purge

### Requirement: Operator visibility
Authenticated administrators SHALL inspect cache settings and statistics, update fields independently, reject negative TTLs, and clear all entries.

#### Scenario: Statistics unavailable
- **WHEN** storage statistics fail
- **THEN** settings are still returned and the failure is logged

