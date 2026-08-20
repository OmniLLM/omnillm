# caching Specification

## Purpose
Defines the opt-in exact-match canonical response cache, including deterministic eligibility, semantic keys, request controls, replay, persistence, expiry, and administration.
## Requirements
### Requirement: Opt-in live configuration
The response cache SHALL remain disabled unless explicitly enabled, SHALL keep enabled state and positive TTL in the durable runtime configuration store, SHALL read those values per request, and SHALL use 60 seconds when no valid positive TTL is configured.

#### Scenario: Configuration changes
- **WHEN** an operator changes cache enablement or TTL
- **THEN** subsequent requests observe the change without restart

#### Scenario: Zero or invalid TTL
- **WHEN** the persisted TTL is absent, zero, negative, or invalid
- **THEN** subsequent cache operations use the 60-second default

#### Scenario: Existing positive TTL
- **WHEN** an operator has configured a positive TTL
- **THEN** subsequent cache operations continue using that configured value rather than the default

### Requirement: Deterministic eligibility
Every otherwise valid request on a supported generation route SHALL be eligible for exact-response caching when the cache is enabled and per-request controls allow caching; eligibility SHALL NOT depend on whether `temperature` or `top_p` is omitted, zero, or nonzero, and streaming intent SHALL not make an otherwise valid request ineligible.

#### Scenario: Sampling controls omitted
- **WHEN** an otherwise valid supported generation request omits `temperature` and `top_p`
- **THEN** the enabled response cache may read or populate an entry for that request

#### Scenario: Temperature omitted
- **WHEN** a request omits temperature
- **THEN** the enabled response cache may read or populate an entry using the omitted value as part of semantic identity

#### Scenario: Nonzero sampling controls
- **WHEN** an otherwise valid supported generation request supplies nonzero `temperature` or a `top_p` below one
- **THEN** the enabled response cache may read or populate an entry using those supplied values as part of semantic identity

#### Scenario: Streaming request
- **WHEN** an otherwise valid supported generation request enables streaming
- **THEN** streaming intent does not prevent cache lookup or complete-response population

### Requirement: Semantic cache key
The key SHALL be stable over every supported generation-affecting canonical field, SHALL distinguish Responses conversation state including `previous_response_id` and forwarded user identity, SHALL exclude streaming mode, transport headers, inbound dialect, non-semantic routing metadata, and provider prompt-cache controls, and SHALL make a request uncacheable when a stable semantic key cannot be produced.

#### Scenario: Cross-mode key
- **WHEN** otherwise identical streaming and non-streaming requests are keyed
- **THEN** they produce the same key

#### Scenario: Responses conversation state
- **WHEN** otherwise identical Responses requests carry different `previous_response_id` values
- **THEN** they produce different keys

#### Scenario: Forwarded user identity
- **WHEN** otherwise identical requests carry different forwarded user identifiers
- **THEN** they produce different keys so one user's upstream-scoped result cannot replay to another

#### Scenario: Prompt-cache placement metadata
- **WHEN** otherwise identical requests differ only in provider prompt-cache breakpoints, native cache key, or retention controls
- **THEN** they produce the same exact-response key

#### Scenario: Generation control changes
- **WHEN** a supported generation-affecting field such as model, content, tools, tool choice, sampling, output limit, stop sequence, structured output, or thinking budget changes
- **THEN** the request produces a different key

#### Scenario: Key serialization fails
- **WHEN** semantic key material cannot be serialized deterministically
- **THEN** no cache read or write occurs and the request proceeds through ordinary provider execution

### Requirement: Route scope
Caching SHALL apply to Chat Completions, Anthropic Messages, and OpenAI Responses generation routes and SHALL not apply to embeddings or token-counting routes.

#### Scenario: Responses request
- **WHEN** an eligible request uses `/v1/responses`
- **THEN** the enabled cache performs the same read and complete-response population policy as the other supported generation routes

#### Scenario: Embeddings request
- **WHEN** a request uses an embeddings route
- **THEN** no exact-response cache read or write occurs

### Requirement: Per-request controls
The cache-control header SHALL support bypass or refresh for skipping reads while allowing writes and off or disable for skipping both.

#### Scenario: Refresh requested
- **WHEN** a client requests refresh
- **THEN** the gateway calls upstream and stores the fresh successful result

### Requirement: Canonical replay
A hit SHALL be re-serialized into the current caller's dialect and streaming mode, SHALL be labeled `X-OmniLLM-Cache: hit` only after successful conversion, and SHALL fall through to ordinary provider execution when cached conversion cannot produce a valid response for the caller.

#### Scenario: Streaming cache hit
- **WHEN** a streaming request hits an entry
- **THEN** the canonical response is synthesized into normal dialect stream events with exactly one terminal event and no duplicate text or tool output

#### Scenario: Responses non-streaming hit
- **WHEN** a non-streaming Responses request hits a compatible canonical entry
- **THEN** the gateway emits a valid Responses object without executing an upstream provider

#### Scenario: Serialization fails
- **WHEN** a stored response cannot be serialized for the caller
- **THEN** the gateway records no served hit, warns without payload disclosure, and falls through to upstream execution

### Requirement: Best-effort population
Cache writes SHALL use the configured Redis response-cache backend, SHALL never fail a successful request, and SHALL exclude errored, incomplete, empty, or unsupported canonical responses. Complete successful streams on every supported generation route SHALL be eligible for population after their terminal event.

#### Scenario: Incomplete stream
- **WHEN** a stream errors or ends without a terminal event
- **THEN** no entry is stored

#### Scenario: Complete Responses stream
- **WHEN** a Responses stream reaches its successful terminal event with cacheable canonical content
- **THEN** its assembled canonical response is stored for later non-streaming or streaming replay

#### Scenario: Redis write failure
- **WHEN** Redis rejects or times out a cache write after upstream execution succeeds
- **THEN** the gateway returns the successful upstream response and records the cache failure without writing to SQLite

### Requirement: Entry lifecycle
Entries SHALL use versioned operator-scoped Redis keys, SHALL expire through native creation-age TTL, SHALL increment hit accounting best-effort, SHALL be atomically replaceable by semantic key, and SHALL support namespace-scoped statistics and complete clear without scanning or deleting unrelated keys.

#### Scenario: Expired read
- **WHEN** Redis expires an entry at its write-time TTL
- **THEN** lookup returns a miss and a later TTL increase does not resurrect that entry

#### Scenario: TTL changes while entries exist
- **WHEN** an operator changes the active TTL
- **THEN** new and refreshed entries use the new TTL while existing entries retain the native expiration assigned when they were written

#### Scenario: Entry replacement
- **WHEN** a successful refresh stores an existing semantic key
- **THEN** payload, creation age, expiration, and hit accounting are replaced atomically

#### Scenario: Namespace isolation
- **WHEN** an administrator inspects or clears response-cache entries
- **THEN** only keys in the configured OmniLLM response-cache namespace are counted or removed

### Requirement: Operator visibility
Authenticated administrators SHALL inspect cache settings, Redis backend availability, and available statistics; SHALL update enabled and TTL fields independently; SHALL reject negative TTLs; and SHALL clear all namespaced response entries.

#### Scenario: Statistics unavailable
- **WHEN** Redis statistics fail
- **THEN** settings and degraded backend availability are still returned with neutral statistics and the failure is logged

#### Scenario: Clear unavailable
- **WHEN** Redis is unavailable during an authenticated clear request
- **THEN** the clear operation reports failure without changing cache enablement or affecting model-serving traffic

### Requirement: Fail-open Redis availability
Redis SHALL be an optional response-cache acceleration dependency, and initialization or runtime failures SHALL bypass cache storage without failing server startup, liveness, or upstream model execution and without falling back to SQLite.

#### Scenario: Redis unavailable at startup
- **WHEN** the configured Redis endpoint cannot be reached or authenticated during startup
- **THEN** OmniLLM starts normally, reports degraded response-cache storage, and continues serving model requests

#### Scenario: Redis read failure
- **WHEN** an eligible request encounters a Redis read error
- **THEN** the cache lookup becomes a miss and the request proceeds through normal provider resolution and upstream execution

#### Scenario: Redis recovers
- **WHEN** Redis becomes available after a startup or runtime outage
- **THEN** bounded recovery probes restore response-cache reads and writes without restarting OmniLLM

### Requirement: Bounded Redis impact
Response-cache Redis operations SHALL use finite connection and command deadlines, avoid unbounded retries, and bypass repeated request-path attempts while the backend is known unavailable.

#### Scenario: Sustained outage
- **WHEN** Redis remains unavailable across multiple eligible requests
- **THEN** requests bypass the open cache circuit without each waiting for a full Redis timeout

#### Scenario: Cache failure logging
- **WHEN** backend availability changes
- **THEN** the system logs the state transition without logging prompt content, response content, or Redis credentials

### Requirement: Exact-response hit accounting
A successfully served exact-response cache hit SHALL be recorded as response-cache traffic, SHALL retain served aggregate input and output token volume when available, and SHALL NOT be classified as a provider prompt-cache hit or miss or report provider cache-read or cache-write token detail.

#### Scenario: Cached response served
- **WHEN** a compatible cached response is successfully emitted to a client
- **THEN** metering records the request with response-cache attribution and unknown provider prompt-cache status

#### Scenario: Cached conversion falls through
- **WHEN** a cache entry is read but cannot be converted for the caller
- **THEN** the failed read is not recorded as a served response-cache hit

### Requirement: Exact-response cache lookup statistics
The response cache SHALL expose namespace-scoped lookup hit and miss counts, the derived lookup hit rate, the start of the current statistics window, and the aggregate canonical-response payload bytes held by live entries. A lookup SHALL count as a hit when a namespaced cache entry is retrieved and as a miss when an attempted namespaced lookup finds no usable entry; disabled or bypassed lookups and backend failures MUST NOT change either count. These lookup statistics MUST remain distinct from provider prompt-cache metrics and from the compatibility `total_hits` live-entry aggregate.

#### Scenario: Cache entry retrieved
- **WHEN** an eligible exact-response lookup retrieves a namespaced cache entry
- **THEN** the namespace lookup-hit count increases once regardless of whether downstream response conversion later succeeds

#### Scenario: Cache entry absent
- **WHEN** an eligible exact-response lookup completes successfully but finds no usable namespaced cache entry
- **THEN** the namespace lookup-miss count increases once

#### Scenario: Lookup not attempted
- **WHEN** exact-response caching is disabled or a request is ineligible and bypasses lookup
- **THEN** neither lookup count changes

#### Scenario: Backend lookup failure
- **WHEN** a Redis failure prevents an attempted lookup from producing a hit or an authoritative miss
- **THEN** neither lookup count changes and model execution continues through the fail-open path

#### Scenario: Live payload usage
- **WHEN** an administrator inspects response-cache statistics
- **THEN** payload usage equals the aggregate encoded canonical-response payload bytes of live entries in the configured namespace

#### Scenario: Hit rate with observations
- **WHEN** at least one counted lookup exists in the current statistics window
- **THEN** lookup hit rate equals lookup hits divided by the sum of lookup hits and lookup misses

#### Scenario: Hit rate without observations
- **WHEN** no counted lookup exists in the current statistics window
- **THEN** lookup hit rate and statistics-window start are reported as unavailable rather than as a fabricated observation

#### Scenario: Cache clear resets statistics
- **WHEN** an administrator successfully clears the response-cache namespace
- **THEN** live entries, payload usage, lookup counts, hit rate, and the current statistics window are reset together

#### Scenario: Compatibility hit aggregate
- **WHEN** response-cache statistics are returned
- **THEN** the existing `total_hits` field remains the sum of hit counts attached to live entries and is not redefined as the cumulative lookup-hit count

