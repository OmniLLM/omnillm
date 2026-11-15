## MODIFIED Requirements

### Requirement: Runtime settings commands
The CLI SHALL read and update supported runtime log levels and response-cache settings, SHALL leave optional settings unchanged when omitted, SHALL reject explicitly negative `--ttl` values before contacting the server, and SHALL report the response-cache backend and availability without exposing Redis credentials.

#### Scenario: Cache TTL omitted
- **WHEN** an operator updates the cache enabled state without `--ttl`
- **THEN** the existing server TTL is not altered

#### Scenario: Negative cache TTL rejected
- **WHEN** an operator invokes `settings set response-cache` with a negative `--ttl` value
- **THEN** the CLI fails validation without sending an update request to the server

#### Scenario: Redis unavailable
- **WHEN** the settings endpoint reports degraded Redis availability
- **THEN** human-readable CLI output identifies the Redis response-cache backend as unavailable while preserving enabled, TTL, entry, and hit output

#### Scenario: Credential-bearing Redis URL
- **WHEN** a configured Redis URL contains authentication information
- **THEN** CLI output, diagnostics, and logs do not expose the username or password

### Requirement: Diagnostics and completion
The CLI SHALL provide configuration/server diagnostics, including non-fatal response-cache backend availability, SHALL direct unconfigured operators to canonical provider login rather than deprecated auth commands, SHALL inspect local diagnostics without creating a database when none exists, and SHALL generate shell completion scripts for supported shells on the command's output stream.

#### Scenario: Diagnostics run
- **WHEN** an operator runs `omnillm doctor`
- **THEN** the CLI checks local configuration, server reachability, and response-cache backend availability and reports actionable findings using canonical CLI commands

#### Scenario: Degraded cache backend
- **WHEN** the gateway is reachable and serving but Redis is unavailable
- **THEN** diagnostics report the response cache as degraded without reporting the gateway itself as unhealthy

#### Scenario: Debug without database
- **WHEN** an operator runs `omnillm debug` before a local SQLite database exists
- **THEN** the CLI reports the missing database without creating a database file on disk

#### Scenario: Shell completion output
- **WHEN** an operator runs `omnillm completion` with one supported shell name
- **THEN** the CLI writes the completion script to its configured output writer and requires the shell argument

### Requirement: CLI prompt-cache usage output
The CLI usage command SHALL display provider prompt-cache hit, miss, and unknown counts and cache read/write token totals in human-readable table output while JSON output SHALL retain the complete raw server response. The usage command SHALL validate `--since` and `--until` as RFC3339 timestamps before querying the server and SHALL advertise `responses` alongside `openai` and `anthropic` for `--api-shape`.

#### Scenario: Usage table with cache data
- **WHEN** an operator requests usage in table format
- **THEN** summary and breakdown output include provider prompt-cache status counts and cache token totals alongside existing request and token totals

#### Scenario: Unknown cache detail
- **WHEN** the server reports unknown prompt-cache status
- **THEN** CLI table output labels it unknown rather than miss

#### Scenario: JSON usage output
- **WHEN** an operator selects JSON output
- **THEN** the CLI prints the server response including cache fields without applying table-only transformations

#### Scenario: Invalid usage time filter
- **WHEN** an operator supplies a non-RFC3339 value to `--since` or `--until`
- **THEN** the CLI fails validation without querying the metering endpoint

## ADDED Requirements

### Requirement: Startup flags reflect active server behavior
The `start` and `restart` commands MUST expose only startup flags backed by active server behavior, SHALL NOT expose dead no-op `--account-type`, `--proxy-env`, `--provider`, or `--console` flags, SHALL apply `--allow-local-endpoints` to server endpoint validation opt-in, and SHALL emit usable client instructions for `--claude-code` in both foreground and background startup modes without emitting placeholder `dummy` credentials.

#### Scenario: Removed no-op startup flags rejected
- **WHEN** an operator passes `--account-type`, `--proxy-env`, `--provider`, or `--console` to `omnillm start` or `omnillm restart`
- **THEN** Cobra rejects the unknown flag rather than silently accepting a no-op option

#### Scenario: Server local-endpoint opt-in
- **WHEN** the server is started with `--allow-local-endpoints`
- **THEN** local or private OpenAI-compatible provider endpoints configured through provider onboarding are permitted while keeping them rejected by default when omitted

#### Scenario: Claude Code guidance in background start
- **WHEN** an operator runs `omnillm start --claude-code` in default background mode
- **THEN** the CLI outputs client base URLs using a reachable loopback address when bound to `0.0.0.0` and does not print `OPENAI_API_KEY=dummy`

### Requirement: Strict CLI argument and parameter validation
Zero-argument CLI commands SHALL reject unexpected positional arguments, `provider priorities --set` SHALL require integer priorities without trailing characters, `virtualmodel` and `config` commands SHALL URL-escape identifier path segments, `virtualmodel update` SHALL distinguish explicitly cleared `--description` flags from omitted flags, and `logs` / `logs tail` SHALL honor `--output json` identically to `--json`.

#### Scenario: Unexpected positional arguments rejected
- **WHEN** an operator passes extra positional arguments to `status`, `status auth`, `usage`, `doctor`, `debug`, `sync-names`, `provider list`, `provider priorities`, `virtualmodel list`, `config list`, `settings get log-level`, `settings get response-cache`, or `settings clear-response-cache`
- **THEN** the CLI exits with an argument validation error before making server calls

#### Scenario: Virtual model description cleared
- **WHEN** an operator runs `omnillm virtualmodel update <id> --description ""`
- **THEN** the CLI sends an empty description rather than preserving the prior description

#### Scenario: Logs honor global JSON output flag
- **WHEN** an operator runs `omnillm logs -o json` or `omnillm logs tail -o json`
- **THEN** the CLI streams one JSON object per log line identically to `--json`

#### Scenario: Provider model and login parameter hygiene
- **WHEN** an operator inspects help or completes arguments for `provider login`, `provider model`, `virtualmodel rename`, `usage`, or `config`
- **THEN** help text matches supported provider behaviors (`--region` for Alibaba, `--endpoint` for endpoint-based providers, integer `<version>` for `model version set`, and `--enable-config-edit` noted on `config set`/`import`) and relevant IDs/flags offer shell completions
