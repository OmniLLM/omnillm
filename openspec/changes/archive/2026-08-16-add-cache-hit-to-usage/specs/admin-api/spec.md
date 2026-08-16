## MODIFIED Requirements

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
