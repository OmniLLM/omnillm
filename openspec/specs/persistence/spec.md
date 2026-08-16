# persistence Specification

## Purpose
Defines OmniLLM durable state in embedded SQLite, schema migration, relational ownership, response and model caches, chats, virtual models, access tokens, metering, and asynchronous writes.
## Requirements
### Requirement: Embedded database
The system SHALL store durable state in `database.sqlite` under the configured directory, creating that directory with owner-only permissions when absent.

#### Scenario: First initialization
- **WHEN** the configuration directory does not exist
- **THEN** it is securely created and the fixed database filename is opened within it

### Requirement: SQLite connection policy
The connection SHALL enable write-ahead logging, foreign keys, a 5000 millisecond busy timeout, and one open and idle connection, and these settings SHALL be effective on every OmniLLM runtime or maintenance connection that opens the durable database.

#### Scenario: Connection initialization
- **WHEN** OmniLLM initializes or a maintenance tool opens the durable database
- **THEN** the effective connection reports write-ahead logging, foreign keys enabled, and a 5000 millisecond busy timeout

#### Scenario: Concurrent write contention
- **WHEN** another database connection temporarily holds a conflicting lock for less than the busy timeout
- **THEN** a metering writer waits for the lock and persists the record after the lock is released rather than failing immediately

### Requirement: Forward-only migrations
Applied integer migration versions SHALL be recorded, pending versions SHALL run in ascending order, and a version SHALL be recorded only after its statements complete.

#### Scenario: Migration fails
- **WHEN** a migration statement fails for a reason other than an already-present column
- **THEN** initialization fails and that version is not recorded

### Requirement: Provider-owned persistence
Provider instances SHALL own credentials, configuration, per-model state/configuration, and model-list caches, creation SHALL persist the provider-instance parent before any owned child record, and deletion SHALL cascade to those records.

#### Scenario: Provider creation
- **WHEN** a new provider authenticates and persists credentials or configuration
- **THEN** its provider-instance parent exists before those owned records are written

#### Scenario: Provider creation fails
- **WHEN** authentication or owned-record persistence fails after reserving a new provider instance
- **THEN** the reserved provider instance and any cascading child records are removed

#### Scenario: Provider deletion
- **WHEN** an instance is deleted
- **THEN** all of its owned credential, configuration, model, and model-cache records are removed

### Requirement: Chat persistence
Chat sessions SHALL own messages whose roles are constrained to user, assistant, or system and whose deletion cascades with the session.

#### Scenario: Unsupported role
- **WHEN** a message uses another role
- **THEN** the database constraint rejects the write

### Requirement: Virtual model persistence
Virtual models SHALL constrain balancing strategy to round-robin, random, priority, or weighted and SHALL own ordered upstream entries with default weight and priority.

#### Scenario: Virtual model deleted
- **WHEN** a virtual model is deleted
- **THEN** all upstream entries are removed

### Requirement: Access token persistence and validation
Access tokens SHALL have unique SHA-256 hashes, display prefixes, enablement and optional expiry, and validation SHALL return only enabled unexpired matches.

#### Scenario: Expired token
- **WHEN** a token's expiry is in the past
- **THEN** validation returns no token record

### Requirement: Request metering
Completed provider calls SHALL record request/model/provider/client/API shape, total prompt tokens, optional uncached and cache read/write token detail, output and total tokens, latency, streaming, status, errors, and time through a bounded asynchronous writer whose enqueue path remains safe during concurrent worker shutdown or restart.

#### Scenario: Metering buffer full
- **WHEN** the asynchronous metering queue is full
- **THEN** the record is dropped with a warning and the user request is unaffected

#### Scenario: Metering enqueue races with shutdown
- **WHEN** a metering enqueue occurs concurrently with worker shutdown or restart
- **THEN** the operation completes without panic, data race, or shutdown deadlock

### Requirement: Access-token usage stamping
Successful access-token use SHALL enqueue last-used timestamp updates without blocking, MAY silently drop an update when its bounded queue is full, and SHALL remain safe during concurrent worker shutdown or restart.

#### Scenario: Full stamp queue
- **WHEN** no queue capacity remains
- **THEN** authentication still succeeds without waiting

#### Scenario: Stamp enqueue races with shutdown
- **WHEN** a token-stamp enqueue occurs concurrently with worker shutdown or restart
- **THEN** the operation completes without panic, data race, or shutdown deadlock

### Requirement: Background worker lifecycle
Database initialization SHALL start metering and token-stamp workers, and shutdown or restart SHALL prevent new enqueue operations from targeting closing queues before draining accepted work.

#### Scenario: Worker restart
- **WHEN** worker startup is requested while workers run
- **THEN** previous workers reject or accept concurrent enqueues safely, stop, and drain accepted work before replacements begin

### Requirement: Initialization required
Access to the global database before successful initialization SHALL fail loudly rather than return an unusable handle.

#### Scenario: Premature database access
- **WHEN** code requests the global database before initialization
- **THEN** the process panics with an initialization directive

### Requirement: Complete database collection reads
Collection-reading persistence operations SHALL return a database iteration or scan error instead of reporting a partial collection as a successful complete result.

#### Scenario: Row iteration fails after valid rows
- **WHEN** a database collection query yields one or more valid rows and then reports an iteration error
- **THEN** the persistence operation returns an error and does not report the partial collection as success

### Requirement: Complete cache snapshot publication
Routing and model-state caches SHALL mark a snapshot loaded and replace the prior published snapshot only after every query, row scan, and iteration for that snapshot completes successfully.

#### Scenario: Virtual-model snapshot load fails
- **WHEN** virtual-model or upstream loading fails before the complete snapshot is available
- **THEN** no partial virtual-model snapshot is published and a later access retries loading

#### Scenario: Provider-instance snapshot load fails
- **WHEN** provider-instance loading fails before the complete snapshot is available
- **THEN** no partial provider-instance snapshot is published and a later access retries loading

#### Scenario: Model-state snapshot load fails
- **WHEN** model-state loading fails before the complete snapshot is available
- **THEN** no partial model-state snapshot is published and a later access retries loading

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

### Requirement: Prompt-cache status derivation
Metering output SHALL derive provider prompt-cache status as `hit` when reported cache-read input tokens are greater than zero, `miss` when a provider explicitly reports zero cache-read input tokens, and `unknown` when no cache-read counter was reported.

#### Scenario: Positive cache read
- **WHEN** a request log contains a positive `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `hit`

#### Scenario: Explicit zero cache read
- **WHEN** a request log contains a reported zero `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `miss`

#### Scenario: Missing cache detail
- **WHEN** a request log has no `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `unknown` and it is not counted as a miss

### Requirement: Prompt-cache usage aggregates
Metering aggregates SHALL report cache-read, cache-write, five-minute-write, one-hour-write, hit, miss, and unknown totals while preserving existing aggregate input, output, and total token semantics.

#### Scenario: Mixed reporting population
- **WHEN** a filtered usage window contains hits, explicit misses, and requests whose provider omitted cache detail
- **THEN** each category is counted independently and the category counts sum to the filtered request count

#### Scenario: Cache token aggregation
- **WHEN** matching request logs contain reported cache-read or cache-write token values
- **THEN** usage aggregates sum those values without replacing unavailable values with inferred tokens

