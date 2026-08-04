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
Completed provider calls SHALL record request/model/provider/client/API shape, tokens, latency, streaming, status, errors, and time through a bounded asynchronous writer.

#### Scenario: Metering buffer full
- **WHEN** the asynchronous metering queue is full
- **THEN** the record is dropped with a warning and the user request is unaffected

### Requirement: Access-token usage stamping
Successful access-token use SHALL enqueue last-used timestamp updates without blocking and MAY silently drop an update when its bounded queue is full.

#### Scenario: Full stamp queue
- **WHEN** no queue capacity remains
- **THEN** authentication still succeeds without waiting

### Requirement: Background worker lifecycle
Database initialization SHALL start metering and token-stamp workers, and shutdown or restart SHALL close and drain existing queues.

#### Scenario: Worker restart
- **WHEN** worker startup is requested while workers run
- **THEN** previous workers are stopped and drained before replacements begin

### Requirement: Initialization required
Access to the global database before successful initialization SHALL fail loudly rather than return an unusable handle.

#### Scenario: Premature database access
- **WHEN** code requests the global database before initialization
- **THEN** the process panics with an initialization directive

