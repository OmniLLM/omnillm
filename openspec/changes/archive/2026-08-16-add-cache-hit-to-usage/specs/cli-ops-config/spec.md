## ADDED Requirements

### Requirement: CLI prompt-cache usage output
The CLI usage command SHALL display provider prompt-cache hit, miss, and unknown counts and cache read/write token totals in human-readable table output while JSON output SHALL retain the complete raw server response.

#### Scenario: Usage table with cache data
- **WHEN** an operator requests usage in table format
- **THEN** summary and breakdown output include provider prompt-cache status counts and cache token totals alongside existing request and token totals

#### Scenario: Unknown cache detail
- **WHEN** the server reports unknown prompt-cache status
- **THEN** CLI table output labels it unknown rather than miss

#### Scenario: JSON usage output
- **WHEN** an operator selects JSON output
- **THEN** the CLI prints the server response including cache fields without applying table-only transformations
