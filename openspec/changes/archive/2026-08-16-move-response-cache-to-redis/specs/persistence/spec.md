## MODIFIED Requirements

### Requirement: Embedded database
The system SHALL store durable state, including response-cache enabled and TTL settings, in `database.sqlite` under the configured directory; SHALL create that directory with owner-only permissions when absent; and SHALL exclude disposable exact-response payloads and hit counters from SQLite.

#### Scenario: First initialization
- **WHEN** the configuration directory does not exist
- **THEN** it is securely created and the fixed database filename is opened within it

#### Scenario: Response-cache configuration
- **WHEN** an operator enables the exact-response cache or changes its TTL
- **THEN** those settings remain durable in SQLite while response payloads are stored only in Redis

### Requirement: Forward-only migrations
Applied integer migration versions SHALL be recorded, pending versions SHALL run in ascending order, a version SHALL be recorded only after its statements complete, and a forward migration SHALL remove the obsolete SQLite exact-response cache schema without altering unrelated durable data.

#### Scenario: Migration fails
- **WHEN** a migration statement fails for a reason other than an already-present column
- **THEN** initialization fails and that version is not recorded

#### Scenario: Upgrade with legacy cached responses
- **WHEN** OmniLLM opens a database containing the legacy `response_cache` table and cached rows
- **THEN** the obsolete table and its disposable rows are removed while cache settings and all unrelated durable records remain intact
