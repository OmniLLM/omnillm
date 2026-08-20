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

The `omnillm` CLI SHALL provide commands to start, stop, and restart the server
and administer providers, provider authentication, provider models, virtual
models, settings, status, logs, usage, synchronization, and Redis-backed
exact-response caching. CLI-managed start and restart operations SHALL run the
server in the background by default and SHALL offer an explicit foreground
mode. Managed lifecycle identity MUST remain valid when the running executable
has been replaced in place, while still rejecting PID reuse and unrelated
processes. The canonical provider workflow SHALL place login and model
operations below `omnillm provider`, and the CLI MUST continue accepting the
previous root `auth`, root `model`, and `provider add` forms as deprecated
hidden compatibility shims.

#### Scenario: Login creates a provider
- **WHEN** an operator runs `omnillm provider login <provider-type>` and the argument does not resolve to an existing provider
- **THEN** the CLI starts authentication for a new instance of that provider type and handles immediate or browser/device authorization through one command

#### Scenario: Login re-authenticates an existing provider
- **WHEN** an operator runs `omnillm provider login <reference>` and the argument resolves by provider ID, name, or alias
- **THEN** the CLI refreshes credentials for that same instance without creating a replacement instance

#### Scenario: Explicit new login
- **WHEN** an operator supplies the new-instance option and a supported provider type to `provider login`
- **THEN** the CLI creates a new provider even if the type text could otherwise resolve as an existing provider reference

#### Scenario: Provider model namespace
- **WHEN** an operator invokes `omnillm provider model list <reference>`
- **THEN** the CLI performs the same provider model-list operation previously available through `omnillm model list`

#### Scenario: Legacy command compatibility
- **WHEN** an automation invokes root `auth`, root `model`, or `provider add`
- **THEN** the invocation retains its prior behavior without those deprecated commands being advertised in ordinary root or provider help

#### Scenario: Live log tail
- **WHEN** an operator runs the log-tail command against a reachable server
- **THEN** the CLI subscribes to the authenticated server-sent-event log stream and prints events as they arrive

#### Scenario: Default Redis endpoint
- **WHEN** the operator starts OmniLLM without a response-cache Redis URL flag or environment value
- **THEN** the server attempts the documented local Redis endpoint without making Redis availability a startup requirement

#### Scenario: Redis URL precedence
- **WHEN** both the response-cache Redis URL flag and its environment variable are set
- **THEN** the explicitly supplied flag value is used

#### Scenario: Redis-enabled Make startup
- **WHEN** an operator needs to start OmniLLM with a Redis response-cache endpoint
- **THEN** the operator invokes `omnillm start` with the canonical environment variable or flag rather than a Make lifecycle target

#### Scenario: Redis-enabled Make startup default
- **WHEN** an operator starts OmniLLM directly without a response-cache Redis URL
- **THEN** the CLI uses the documented local Redis endpoint without requiring a Make wrapper

#### Scenario: Redis startup help
- **WHEN** an operator reads current startup documentation
- **THEN** it documents the direct `omnillm` Redis configuration and does not advertise a Redis-specific Make target

#### Scenario: Stop a running server
- **WHEN** an operator invokes `omnillm stop` and lifecycle state identifies a live OmniLLM server started by the same operator
- **THEN** the CLI requests graceful termination, waits for the process to exit within a bounded interval, and removes the lifecycle state

#### Scenario: Restart after executable replacement
- **WHEN** an operator reinstalls or atomically replaces `omnillm` while its managed background server remains running and then invokes `omnillm restart`
- **THEN** the CLI recognizes and terminates that same live process before starting its replacement on the requested address

#### Scenario: Reused PID is not managed
- **WHEN** lifecycle state refers to a PID whose process start ID or normalized executable path differs from the recorded identity
- **THEN** the CLI treats the state as stale and does not signal that process

#### Scenario: Stop without a managed server
- **WHEN** an operator invokes `omnillm stop` and no valid live OmniLLM server is identified
- **THEN** the CLI returns an actionable error and does not signal another process

#### Scenario: Restart a running server
- **WHEN** an operator invokes `omnillm restart` and lifecycle state identifies a live OmniLLM server
- **THEN** the CLI gracefully stops that process, starts a replacement in the background using the startup options supplied to `restart`, waits for readiness, and returns

#### Scenario: Restart without a running server
- **WHEN** an operator invokes `omnillm restart` and no live managed server is identified
- **THEN** the CLI starts the server in the background using the startup options supplied to `restart`, waits for readiness, and returns

#### Scenario: Start records lifecycle state
- **WHEN** the managed server begins serving successfully
- **THEN** it atomically records sufficient operator-local lifecycle identity for a later stop invocation and removes that state when the server exits

#### Scenario: Background start succeeds
- **WHEN** an operator invokes `omnillm start` and the child server successfully binds and registers lifecycle state
- **THEN** the CLI reports the managed server identity and address, exits successfully, and leaves the server running independently in the background

#### Scenario: Background start fails
- **WHEN** the child server fails before readiness or does not become ready within the bounded startup interval
- **THEN** the CLI returns an actionable error, reaps or terminates the child as needed, and leaves no live lifecycle state

#### Scenario: Explicit foreground mode
- **WHEN** an operator invokes `omnillm start --foreground` or `omnillm restart --foreground`
- **THEN** the server runs attached to the invoking process until shutdown

#### Scenario: Existing foreground behavior
- **WHEN** an existing operator workflow requires the prior attached execution behavior
- **THEN** invoking `omnillm start --foreground` preserves that behavior until server shutdown

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

### Requirement: Interactive provider rename

The CLI SHALL prompt for provider rename metadata when invoked interactively
without rename fields and MUST retain non-interactive flag validation for
automation.

#### Scenario: Interactive provider rename without metadata flags

- **WHEN** an operator invokes `omnillm provider rename` from an interactive
  terminal, selects or supplies a provider, and supplies neither `--name` nor
  `--alias`
- **THEN** the CLI prompts for a new display name and alias and submits the
  non-empty values after at least one value is provided

#### Scenario: Interactive provider rename leaves both fields empty

- **WHEN** the interactive rename prompts produce neither a display name nor an
  alias
- **THEN** the CLI returns an actionable validation error without sending a
  rename request

#### Scenario: Flag-driven provider rename

- **WHEN** an operator supplies `--name`, `--alias`, or its compatible hidden
  spelling to `provider rename`
- **THEN** the CLI uses the supplied fields without prompting and preserves the
  existing conflict validation

#### Scenario: Non-interactive provider rename without metadata flags

- **WHEN** automation invokes `provider rename` without `--name` or `--alias`
- **THEN** the CLI fails validation without prompting or sending a rename
  request

### Requirement: Canonical command binary

Repository build, installation, and documentation workflows MUST produce and
advertise `omnillm` as the sole supported Go command binary and MUST NOT build,
install, or advertise an `omniproxy` command binary.

#### Scenario: Build and install commands

- **WHEN** an operator runs a supported repository Go build or installation workflow
- **THEN** the workflow produces or installs `omnillm` without producing or installing `omniproxy`

#### Scenario: Current command documentation

- **WHEN** an operator reads current repository usage or architecture documentation
- **THEN** command examples and entrypoint descriptions identify `omnillm` and do not offer `omniproxy`

#### Scenario: Go package discovery

- **WHEN** Go tooling enumerates buildable packages in the repository
- **THEN** no `cmd/omniproxy` command package is present

### Requirement: Canonical Go toolchain

Current repository module, container, continuous-integration, and
documentation configuration MUST consistently require Go 1.27 or newer.

#### Scenario: Repository Go build

- **WHEN** a contributor, CI job, or container build resolves the repository Go toolchain
- **THEN** it uses the Go 1.27 module requirement rather than an older Go release

#### Scenario: Current development documentation

- **WHEN** a contributor reads the current setup or architecture documentation
- **THEN** Go 1.27 or newer is identified as the supported toolchain

### Requirement: Minimal Make orchestration

The repository Makefile MUST be limited to building, installing, and
uninstalling the canonical `omnillm` binary and orchestrating desktop sidecar
workflows, and MUST NOT wrap OmniLLM lifecycle, Bun development, validation,
release, or container commands.

#### Scenario: Server lifecycle

- **WHEN** an operator starts, stops, restarts, inspects, or follows logs for the packaged gateway
- **THEN** current documentation directs the operator to `omnillm` rather than a Make target

#### Scenario: Canonical binary build

- **WHEN** a contributor runs `make build`
- **THEN** the root Go package is compiled as `omnillm` under `.build/bin` for the host platform

#### Scenario: Canonical binary installation

- **WHEN** a contributor runs `make install`
- **THEN** the root Go package is installed through `go install .` using the configured Go installation path

#### Scenario: Canonical binary uninstallation

- **WHEN** a contributor runs `make uninstall`
- **THEN** only the installed `omnillm` executable and any stale legacy `omniproxy` executable are removed from Go's effective binary installation directory and absent executables are treated as success

#### Scenario: Retained Make surface

- **WHEN** a contributor inspects public Make targets
- **THEN** only `build`, `install`, `uninstall`, `build-desktop-sidecar`, `build-desktop`, and `desktop-dev` are available

#### Scenario: Desktop workflows

- **WHEN** a contributor invokes a retained desktop Make target
- **THEN** `build-desktop-sidecar`, `build-desktop`, and `desktop-dev` continue to provide cross-platform desktop orchestration

#### Scenario: Direct development tools

- **WHEN** a contributor tests, lints, releases, or runs a container outside desktop packaging
- **THEN** current documentation uses the direct Bun, release-script, or Docker command

