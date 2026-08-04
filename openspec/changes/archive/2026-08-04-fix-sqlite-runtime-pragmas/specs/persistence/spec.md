## MODIFIED Requirements

### Requirement: SQLite connection policy
The connection SHALL enable write-ahead logging, foreign keys, a 5000 millisecond busy timeout, and one open and idle connection, and these settings SHALL be effective on every OmniLLM runtime or maintenance connection that opens the durable database.

#### Scenario: Connection initialization
- **WHEN** OmniLLM initializes or a maintenance tool opens the durable database
- **THEN** the effective connection reports write-ahead logging, foreign keys enabled, and a 5000 millisecond busy timeout

#### Scenario: Concurrent write contention
- **WHEN** another database connection temporarily holds a conflicting lock for less than the busy timeout
- **THEN** a metering writer waits for the lock and persists the record after the lock is released rather than failing immediately

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
