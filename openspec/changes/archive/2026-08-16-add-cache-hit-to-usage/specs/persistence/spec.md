## ADDED Requirements

### Requirement: Prompt-cache status derivation
Metering output SHALL derive provider prompt-cache status as `hit` when reported cache-read input tokens are greater than zero, `miss` when a provider explicitly reports zero cache-read input tokens, and `unknown` when no cache-read counter was reported.

#### Scenario: Positive cache read
- **WHEN** a request log contains a positive `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `hit`

#### Scenario: Explicit zero cache read
- **WHEN** a request log contains a reported zero `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `miss`

#### Scenario: Missing cache detail
- **WHEN** a request log has no `cache_read_input_tokens` value
- **THEN** its prompt-cache status is `unknown` and it is not counted as a miss

### Requirement: Prompt-cache usage aggregates
Metering aggregates SHALL report cache-read, cache-write, five-minute-write, one-hour-write, hit, miss, and unknown totals while preserving existing aggregate input, output, and total token semantics.

#### Scenario: Mixed reporting population
- **WHEN** a filtered usage window contains hits, explicit misses, and requests whose provider omitted cache detail
- **THEN** each category is counted independently and the category counts sum to the filtered request count

#### Scenario: Cache token aggregation
- **WHEN** matching request logs contain reported cache-read or cache-write token values
- **THEN** usage aggregates sum those values without replacing unavailable values with inferred tokens
