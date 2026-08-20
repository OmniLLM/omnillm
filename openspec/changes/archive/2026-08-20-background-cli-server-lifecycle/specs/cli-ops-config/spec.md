## MODIFIED Requirements

### Requirement: Server operation and administration

The CLI and repository operator workflows SHALL provide commands to start,
stop, and restart the server and administer providers, provider authentication,
provider models, virtual models, settings, status, logs, usage,
synchronization, and Redis-backed exact-response caching. CLI-managed start and
restart operations SHALL run the server in the background by default and SHALL
offer an explicit foreground mode. The canonical provider workflow SHALL place
login and model operations below `omnillm provider`, and the CLI MUST continue
accepting the previous root `auth`, root `model`, and `provider add` forms as
deprecated hidden compatibility shims.

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
- **WHEN** an operator invokes the documented Redis-enabled Make startup target with `OMNILLM_RESPONSE_CACHE_REDIS_URL` inherited from the process environment
- **THEN** the managed OmniLLM backend receives that URL as its response-cache Redis endpoint without evaluating the URL as recipe shell source while the normal backend, frontend, host, and port startup workflow is preserved

#### Scenario: Redis-enabled Make startup default
- **WHEN** an operator invokes the Redis-enabled Make startup target without a response-cache Redis URL in the environment
- **THEN** the backend uses the documented local Redis endpoint

#### Scenario: Redis startup help
- **WHEN** an operator runs the Make help target
- **THEN** the Redis-enabled startup target, canonical Redis URL environment variable, local default, and runnable examples are displayed

#### Scenario: Stop a running server
- **WHEN** an operator invokes `omnillm stop` and lifecycle state identifies a live OmniLLM server started by the same operator
- **THEN** the CLI requests graceful termination, waits for the process to exit within a bounded interval, and removes the lifecycle state

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
