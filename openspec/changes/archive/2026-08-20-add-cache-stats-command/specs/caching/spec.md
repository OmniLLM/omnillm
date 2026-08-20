## ADDED Requirements

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
