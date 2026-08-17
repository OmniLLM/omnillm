## MODIFIED Requirements

### Requirement: Server operation and administration
The CLI and repository operator workflows SHALL provide commands to start the server and administer provider authentication, providers, models, virtual models, settings, status, logs, usage, synchronization, and Redis-backed exact-response caching.

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
