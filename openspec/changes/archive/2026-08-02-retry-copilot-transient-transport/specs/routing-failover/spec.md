## ADDED Requirements

### Requirement: Correlatable provider failure diagnostics
When every candidate has failed and the gateway emits its terminal provider-failure error log, that log SHALL include the request identifier, the failing provider instance, and the upstream model, in addition to the underlying error. Client-disconnect cases SHALL retain their existing informational handling and SHALL NOT be logged as provider failures.

#### Scenario: All providers failed
- **WHEN** the gateway records a terminal provider failure after all candidates are exhausted
- **THEN** the error log carries the request identifier, provider instance, and upstream model so it can be correlated with the gateway request log

#### Scenario: Client canceled
- **WHEN** the terminal failure is a client disconnect
- **THEN** it remains an informational log and no provider-failure error is emitted
