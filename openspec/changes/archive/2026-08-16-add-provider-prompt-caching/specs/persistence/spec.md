## ADDED Requirements

### Requirement: Prompt-cache metering detail
Completed provider calls SHALL persist optional uncached-input, cache-read-input, cache-write-input, five-minute-write, and one-hour-write counters alongside the existing aggregate token fields.

#### Scenario: Detailed cache usage recorded
- **WHEN** a successful provider call reports cache-read and cache-write detail
- **THEN** the request log stores those counters while `input_tokens` remains total prompt input and `total_tokens` remains total prompt input plus output

#### Scenario: Provider omits cache usage
- **WHEN** a provider reports only aggregate input and output tokens
- **THEN** aggregate metering remains correct and unavailable cache detail is not inferred

### Requirement: Backward-compatible cache usage migration
The database migration SHALL add prompt-cache metering fields without losing existing request logs, changing existing aggregate values, or preventing an older database from opening after upgrade.

#### Scenario: Upgrade populated database
- **WHEN** OmniLLM opens a database created before prompt-cache metering fields existed
- **THEN** all prior request-log rows remain queryable with their original aggregate token values and neutral cache-detail defaults

## MODIFIED Requirements

### Requirement: Request metering
Completed provider calls SHALL record request/model/provider/client/API shape, total prompt tokens, optional uncached and cache read/write token detail, output and total tokens, latency, streaming, status, errors, and time through a bounded asynchronous writer whose enqueue path remains safe during concurrent worker shutdown or restart.

#### Scenario: Metering buffer full
- **WHEN** the asynchronous metering queue is full
- **THEN** the record is dropped with a warning and the user request is unaffected

#### Scenario: Metering enqueue races with shutdown
- **WHEN** a metering enqueue occurs concurrently with worker shutdown or restart
- **THEN** the operation completes without panic, data race, or shutdown deadlock
