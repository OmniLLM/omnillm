## ADDED Requirements

### Requirement: Metering prompt-cache visibility
The Metering workspace SHALL display provider prompt-cache status and cache token details for individual requests and SHALL display cache hit, miss, unknown, read-token, and write-token aggregates for the selected filters.

#### Scenario: Cache-hit request row
- **WHEN** a metering row has prompt-cache status `hit`
- **THEN** the Usage page displays a distinct hit indicator and its reported cache-read token count

#### Scenario: Unknown cache status
- **WHEN** a metering row has prompt-cache status `unknown`
- **THEN** the Usage page displays unknown rather than presenting the request as a cache miss

#### Scenario: Responsive cache columns
- **WHEN** cache details are shown on a narrow viewport or within a dense table
- **THEN** the operator can still inspect the status and token values without losing existing request identity and total usage fields

### Requirement: Prompt-cache terminology
The Metering workspace SHALL label provider prompt-cache status separately from OmniLLM exact-response-cache behavior and SHALL NOT describe an unknown provider counter as a miss.

#### Scenario: Provider prompt-cache legend
- **WHEN** an operator views cache metrics
- **THEN** labels or explanatory text identify them as provider prompt-cache usage and define hit, miss, and unknown consistently
