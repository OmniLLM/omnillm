# cli-ops-config Specification

## Purpose
Defines the OmniLLM command-line and operational configuration contracts for starting the gateway, administering it remotely, diagnosing it, and integrating external tools.
## Requirements
### Requirement: CLI command surface
The `omnillm` CLI SHALL group server, provider, administration, and troubleshooting commands and SHALL report one formatted error without automatically printing usage on command failure.

#### Scenario: Command failure
- **WHEN** a subcommand returns an error
- **THEN** the CLI writes `Error: <message>` to standard error and exits with status 1

### Requirement: Server address resolution
The CLI SHALL resolve the server from `--server`, then `OMNILLM_SERVER`, then `http://127.0.0.1:5000`, and SHALL remove trailing slashes.

#### Scenario: Flag overrides environment
- **WHEN** both the flag and environment variable are set
- **THEN** outbound requests use the flag value

### Requirement: API-key resolution
The CLI SHALL resolve its admin API key from `--api-key` and then `OMNILLM_API_KEY` and SHALL transmit a resolved key as an Authorization bearer token.

#### Scenario: No configured CLI key
- **WHEN** neither source supplies a key
- **THEN** the request is sent without an Authorization header

### Requirement: Output formats
Read operations SHALL support table and JSON output, default to table, and return raw server JSON when JSON is selected.

#### Scenario: JSON output selected
- **WHEN** an operator uses `--output json`
- **THEN** the CLI prints the server JSON without table rendering

### Requirement: Server operation and administration
The CLI SHALL provide commands to start the server and administer provider authentication, providers, models, virtual models, settings, status, logs, usage, and synchronization.

#### Scenario: Live log tail
- **WHEN** an operator runs the log-tail command against a reachable server
- **THEN** the CLI subscribes to the authenticated server-sent-event log stream and prints events as they arrive

### Requirement: External tool configuration commands
The CLI SHALL list, read, write, import, and back up known external tool configurations through the admin API, requiring exactly one content source for writes.

#### Scenario: Ambiguous write source
- **WHEN** a config-set invocation supplies both `--file` and `--stdin`, or neither
- **THEN** the CLI fails validation without contacting the server

### Requirement: Runtime settings commands
The CLI SHALL read and update supported runtime log levels and response-cache settings and SHALL leave optional settings unchanged when omitted.

#### Scenario: Cache TTL omitted
- **WHEN** an operator updates the cache enabled state without `--ttl`
- **THEN** the existing server TTL is not altered

### Requirement: Diagnostics and completion
The CLI SHALL provide configuration/server diagnostics and generate shell completion scripts for supported shells.

#### Scenario: Diagnostics run
- **WHEN** an operator runs `omnillm doctor`
- **THEN** the CLI checks local configuration and server reachability and reports actionable findings

### Requirement: CLI prompt-cache usage output
The CLI usage command SHALL display provider prompt-cache hit, miss, and unknown counts and cache read/write token totals in human-readable table output while JSON output SHALL retain the complete raw server response.

#### Scenario: Usage table with cache data
- **WHEN** an operator requests usage in table format
- **THEN** summary and breakdown output include provider prompt-cache status counts and cache token totals alongside existing request and token totals

#### Scenario: Unknown cache detail
- **WHEN** the server reports unknown prompt-cache status
- **THEN** CLI table output labels it unknown rather than miss

#### Scenario: JSON usage output
- **WHEN** an operator selects JSON output
- **THEN** the CLI prints the server response including cache fields without applying table-only transformations

