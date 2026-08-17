## MODIFIED Requirements

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
