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

The CLI and repository operator workflows SHALL provide commands to start the
server and administer providers, provider authentication, provider models,
virtual models, settings, status, logs, usage, synchronization, and Redis-backed
exact-response caching. The canonical provider workflow SHALL place login and
model operations below `omnillm provider`, and the CLI MUST continue accepting
the previous root `auth`, root `model`, and `provider add` forms as deprecated
hidden compatibility shims.

#### Scenario: Login creates a provider

- **WHEN** an operator runs `omnillm provider login <provider-type>` and the
  argument does not resolve to an existing provider
- **THEN** the CLI starts authentication for a new instance of that provider
  type and handles immediate or browser/device authorization through one command

#### Scenario: Login re-authenticates an existing provider

- **WHEN** an operator runs `omnillm provider login <reference>` and the
  argument resolves by provider ID, name, or alias
- **THEN** the CLI refreshes credentials for that same instance without creating
  a replacement instance

#### Scenario: Explicit new login

- **WHEN** an operator supplies the new-instance option and a supported provider
  type to `provider login`
- **THEN** the CLI creates a new provider even if the type text could otherwise
  resolve as an existing provider reference

#### Scenario: Provider model namespace

- **WHEN** an operator invokes `omnillm provider model list <reference>`
- **THEN** the CLI performs the same provider model-list operation previously
  available through `omnillm model list`

#### Scenario: Legacy command compatibility

- **WHEN** an automation invokes root `auth`, root `model`, or `provider add`
- **THEN** the invocation retains its prior behavior without those deprecated
  commands being advertised in ordinary root or provider help

#### Scenario: Live log tail

- **WHEN** an operator runs the log-tail command against a reachable server
- **THEN** the CLI subscribes to the authenticated server-sent-event log stream
  and prints events as they arrive

#### Scenario: Default Redis endpoint

- **WHEN** the operator starts OmniLLM without a response-cache Redis URL flag
  or environment value
- **THEN** the server attempts the documented local Redis endpoint without
  making Redis availability a startup requirement

#### Scenario: Redis URL precedence

- **WHEN** both the response-cache Redis URL flag and its environment variable
  are set
- **THEN** the explicitly supplied flag value is used

#### Scenario: Redis-enabled Make startup

- **WHEN** an operator invokes the documented Redis-enabled Make startup target
  with `OMNILLM_RESPONSE_CACHE_REDIS_URL` inherited from the process environment
- **THEN** the managed OmniLLM backend receives that URL as its response-cache
  Redis endpoint without evaluating the URL as recipe shell source while the
  normal backend, frontend, host, and port startup workflow is preserved

#### Scenario: Redis-enabled Make startup default

- **WHEN** an operator invokes the Redis-enabled Make startup target without a
  response-cache Redis URL in the environment
- **THEN** the backend uses the documented local Redis endpoint

#### Scenario: Redis startup help

- **WHEN** an operator runs the Make help target
- **THEN** the Redis-enabled startup target, canonical Redis URL environment
  variable, local default, and runnable examples are displayed

### Requirement: External tool configuration commands
The CLI SHALL list, read, write, import, and back up known external tool configurations through the admin API, requiring exactly one content source for writes.

#### Scenario: Ambiguous write source
- **WHEN** a config-set invocation supplies both `--file` and `--stdin`, or neither
- **THEN** the CLI fails validation without contacting the server

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

