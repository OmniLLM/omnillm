## MODIFIED Requirements

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
