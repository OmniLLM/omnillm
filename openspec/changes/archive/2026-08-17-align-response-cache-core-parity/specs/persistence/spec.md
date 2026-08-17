## MODIFIED Requirements

### Requirement: Request metering
Completed provider calls and successfully served exact-response cache hits SHALL record request/model/source/client/API shape, aggregate prompt and output token volume when available, latency, streaming, status, errors, and time through a bounded asynchronous writer whose enqueue path remains safe during concurrent worker shutdown or restart. Provider calls SHALL retain optional uncached and provider cache read/write detail. Exact-response hits SHALL use response-cache attribution and SHALL leave provider cache read/write detail unavailable.

#### Scenario: Metering buffer full
- **WHEN** the asynchronous metering queue is full
- **THEN** the record is dropped with a warning and the user request is unaffected

#### Scenario: Metering enqueue races with shutdown
- **WHEN** a metering enqueue occurs concurrently with worker shutdown or restart
- **THEN** the operation completes without panic, data race, or shutdown deadlock

#### Scenario: Provider result metered
- **WHEN** an upstream provider call completes
- **THEN** metering records the provider identity, aggregate usage, and only the provider cache detail that was reported

#### Scenario: Exact-response hit metered
- **WHEN** a canonical cache entry is successfully served without provider execution
- **THEN** metering records response-cache attribution and aggregate served token volume while provider cache-read and cache-write fields remain absent

#### Scenario: Cache read cannot be served
- **WHEN** a cache entry is read but replay conversion fails and provider execution follows
- **THEN** metering does not record a served exact-response hit for the failed replay

### Requirement: Prompt-cache status derivation
Metering output SHALL derive provider prompt-cache status as `hit` when a completed provider call reported cache-read input tokens greater than zero, `miss` when a completed provider call explicitly reported zero cache-read input tokens, and `unknown` when no provider cache-read counter was reported, including exact-response cache hits that performed no provider call.

#### Scenario: Positive cache read
- **WHEN** a provider request log contains a positive `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `hit`

#### Scenario: Explicit zero cache read
- **WHEN** a provider request log contains a reported zero `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `miss`

#### Scenario: Missing cache detail
- **WHEN** a request log has no `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `unknown` and it is not counted as a miss

#### Scenario: Exact-response traffic
- **WHEN** a request is served from the exact-response cache
- **THEN** its provider prompt-cache status is `unknown` regardless of cache detail retained inside the replayed canonical payload
