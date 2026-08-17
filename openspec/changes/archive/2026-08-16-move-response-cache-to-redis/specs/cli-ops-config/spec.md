## MODIFIED Requirements

### Requirement: Server operation and administration
The CLI SHALL provide commands to start the server and administer provider authentication, providers, models, virtual models, settings, status, logs, usage, synchronization, and Redis-backed exact-response caching.

#### Scenario: Live log tail
- **WHEN** an operator runs the log-tail command against a reachable server
- **THEN** the CLI subscribes to the authenticated server-sent-event log stream and prints events as they arrive

#### Scenario: Default Redis endpoint
- **WHEN** the operator starts OmniLLM without a response-cache Redis URL flag or environment value
- **THEN** the server attempts the documented local Redis endpoint without making Redis availability a startup requirement

#### Scenario: Redis URL precedence
- **WHEN** both the response-cache Redis URL flag and its environment variable are set
- **THEN** the explicitly supplied flag value is used

### Requirement: Runtime settings commands
The CLI SHALL read and update supported runtime log levels and response-cache settings, SHALL leave optional settings unchanged when omitted, and SHALL report the response-cache backend and availability without exposing Redis credentials.

#### Scenario: Cache TTL omitted
- **WHEN** an operator updates the cache enabled state without `--ttl`
- **THEN** the existing server TTL is not altered

#### Scenario: Redis unavailable
- **WHEN** the settings endpoint reports degraded Redis availability
- **THEN** human-readable CLI output identifies the Redis response-cache backend as unavailable while preserving enabled, TTL, entry, and hit output

#### Scenario: Credential-bearing Redis URL
- **WHEN** a configured Redis URL contains authentication information
- **THEN** CLI output, diagnostics, and logs do not expose the username or password

### Requirement: Diagnostics and completion
The CLI SHALL provide configuration/server diagnostics, including non-fatal response-cache backend availability, and SHALL generate shell completion scripts for supported shells.

#### Scenario: Diagnostics run
- **WHEN** an operator runs `omnillm doctor`
- **THEN** the CLI checks local configuration, server reachability, and response-cache backend availability and reports actionable findings

#### Scenario: Degraded cache backend
- **WHEN** the gateway is reachable and serving but Redis is unavailable
- **THEN** diagnostics report the response cache as degraded without reporting the gateway itself as unhealthy
