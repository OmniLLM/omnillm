## MODIFIED Requirements

### Requirement: Opt-in live configuration
The response cache SHALL remain disabled unless explicitly enabled, SHALL keep enabled state and positive TTL in the durable runtime configuration store, and SHALL read those values per request while defaulting TTL to one hour.

#### Scenario: Configuration changes
- **WHEN** an operator changes cache enablement or TTL
- **THEN** subsequent requests observe the change without restart

#### Scenario: Zero or invalid TTL
- **WHEN** the persisted TTL is absent, zero, negative, or invalid
- **THEN** subsequent cache operations use the one-hour default

### Requirement: Best-effort population
Cache writes SHALL use the configured Redis response-cache backend, SHALL never fail a successful request, and SHALL exclude errored, incomplete, empty, or unsupported canonical responses.

#### Scenario: Incomplete stream
- **WHEN** a stream errors or ends without a terminal event
- **THEN** no entry is stored

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

## ADDED Requirements

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
