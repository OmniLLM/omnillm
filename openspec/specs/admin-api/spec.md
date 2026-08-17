# admin-api Specification

## Purpose
Defines the HTTP administration surface used to operate OmniLLM providers, virtual models, settings, metering, sessions, access tokens, and external tool configuration.
## Requirements
### Requirement: Administrative authentication
The system SHALL protect `/api/admin` endpoints with bearer or `x-api-key` authentication except for explicitly public information and OAuth flow endpoints.

#### Scenario: Protected request without credentials
- **WHEN** a client calls a protected admin endpoint without a valid credential
- **THEN** the system responds with HTTP 401 and does not invoke the handler

#### Scenario: Public information request
- **WHEN** a client calls `GET /api/admin/info` without credentials
- **THEN** the system returns server version, port, backend, uptime, and authentication status

### Requirement: Provider lifecycle management
The system SHALL expose authenticated operations to list, create or authenticate, configure, prioritize, activate, deactivate, rename, and delete provider instances and to manage their model catalogs.

#### Scenario: Provider deactivation
- **WHEN** an authenticated operator deactivates an existing provider
- **THEN** the provider is excluded from the active provider set

#### Scenario: Model catalog refresh
- **WHEN** an authenticated operator requests a provider model refresh
- **THEN** the system fetches and returns the refreshed upstream catalog

### Requirement: Runtime status and settings
The system SHALL report active providers and enabled model counts and SHALL allow authenticated operators to manage runtime log level and Redis-backed response-cache settings, statistics, availability, and namespace-scoped clearing.

#### Scenario: Runtime log-level update
- **WHEN** an authenticated operator sets a supported log level
- **THEN** the level takes effect without a restart and is returned by the settings endpoint

#### Scenario: Cache settings response
- **WHEN** an authenticated operator reads response-cache settings
- **THEN** the response includes the existing enabled, TTL, entry-count, and total-hit fields plus additive backend and availability fields

#### Scenario: Cache statistics unavailable
- **WHEN** Redis statistics cannot be read
- **THEN** the endpoint returns HTTP 200 with durable settings, neutral statistics, backend marked unavailable, and no Redis credentials

#### Scenario: Cache clear
- **WHEN** an authenticated operator clears the response cache while Redis is available
- **THEN** only response entries in the configured OmniLLM namespace are removed and success with the removed count is reported

#### Scenario: Cache clear unavailable
- **WHEN** an authenticated operator clears the response cache while Redis is unavailable
- **THEN** the endpoint returns a server error and does not claim that cached entries were removed

### Requirement: Virtual model management
The system SHALL provide authenticated create, read, update, rename, list, and delete operations for virtual models and their upstream definitions.

#### Scenario: Invalid virtual model
- **WHEN** a create request omits its identifier, name, or an upstream model identifier
- **THEN** the system rejects the request with HTTP 400 without persisting it

### Requirement: Metering and log access
The system SHALL expose authenticated raw and aggregate metering views with model, provider, client, API-shape, time, and provider prompt-cache filters, including nullable cache token detail and derived `hit`, `miss`, or `unknown` prompt-cache status, plus a server-sent-event stream of live logs.

#### Scenario: Filtered metering query
- **WHEN** a metering request supplies supported dimension, time, or prompt-cache-status filters
- **THEN** only matching request records contribute to the response

#### Scenario: Raw metering cache detail
- **WHEN** a raw metering row is returned
- **THEN** it includes nullable uncached, cache-read, cache-write, five-minute-write, and one-hour-write token fields and its derived prompt-cache status

#### Scenario: Aggregate metering cache detail
- **WHEN** an aggregate metering endpoint is requested
- **THEN** it includes cache token totals and hit, miss, and unknown request counts for the selected window or grouping

#### Scenario: Live log subscription
- **WHEN** an authenticated client opens the log stream
- **THEN** the connection remains open and receives newly emitted log events

### Requirement: Chat session management
The system SHALL provide authenticated operations to create, list, read, update, append messages to, and delete stored chat sessions.

#### Scenario: Delete all sessions
- **WHEN** an authenticated operator deletes the chat-session collection
- **THEN** subsequent session listing returns an empty collection

### Requirement: Access token administration
The system SHALL create, list, and revoke access tokens while storing only token hashes and returning a new plaintext token only when it is created.

#### Scenario: Revoked access token
- **WHEN** an operator deletes an access token and it is presented on a later request
- **THEN** authentication rejects the token

### Requirement: External tool configuration management
The system SHALL list, read, write, import, and back up supported external tool configuration files only when privileged configuration editing is enabled.

#### Scenario: Configuration editing disabled
- **WHEN** a client attempts to write or import a configuration while editing is disabled
- **THEN** the system responds with HTTP 403 without modifying the file

#### Scenario: Malformed structured configuration
- **WHEN** JSON-backed tool configuration content is invalid JSON
- **THEN** the system responds with HTTP 400 and preserves the existing file

#### Scenario: Secure configuration write
- **WHEN** valid configuration is written for a known tool
- **THEN** parent directories and the file are created with owner-only permissions

### Requirement: Bounded configuration imports
The administration API SHALL enforce a finite upload limit before parsing multipart configuration imports and SHALL return HTTP 413 without modifying the target file when the request or uploaded configuration exceeds that limit.

#### Scenario: Configuration upload exceeds the limit
- **WHEN** an authenticated operator uploads a configuration larger than the configured import limit
- **THEN** the system returns HTTP 413 and preserves the existing target file

#### Scenario: Configuration upload remains within the limit
- **WHEN** an authenticated operator uploads a valid configuration at or below the configured import limit
- **THEN** the existing validation and secure-write behavior applies unchanged

