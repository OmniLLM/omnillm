## ADDED Requirements

### Requirement: Exact-response cache statistics command
The CLI SHALL provide `omnillm cache stats` as an administration command for the local exact-response cache. Table output SHALL identify configuration and backend availability and display live entry count, payload usage in bytes with a human-readable equivalent, lookup hits, lookup misses, lookup hit rate, and statistics-window start. JSON output SHALL return the complete unmodified response-cache settings response. The command MUST distinguish these local exact-response lookup statistics from provider prompt-cache usage reported by `omnillm usage`.

#### Scenario: Statistics table
- **WHEN** an operator runs `omnillm cache stats` using the default table output
- **THEN** the CLI renders response-cache enabled state, TTL, backend availability, entries, payload usage, lookup hits, lookup misses, hit rate, and statistics-window start

#### Scenario: Statistics JSON
- **WHEN** an operator runs `omnillm cache stats --output json`
- **THEN** the CLI prints the raw authenticated response-cache settings JSON without table transformation

#### Scenario: No observed lookups
- **WHEN** the server reports no current statistics window or calculable hit rate
- **THEN** table output labels those values as unavailable rather than displaying a zero rate or fabricated timestamp

#### Scenario: Degraded backend
- **WHEN** the server reports that Redis statistics are unavailable
- **THEN** table output identifies the Redis backend as unavailable and displays neutral numeric statistics without failing the command
