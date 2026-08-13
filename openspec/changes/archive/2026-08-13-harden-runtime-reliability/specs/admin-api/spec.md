## ADDED Requirements

### Requirement: Bounded configuration imports
The administration API SHALL enforce a finite upload limit before parsing multipart configuration imports and SHALL return HTTP 413 without modifying the target file when the request or uploaded configuration exceeds that limit.

#### Scenario: Configuration upload exceeds the limit
- **WHEN** an authenticated operator uploads a configuration larger than the configured import limit
- **THEN** the system returns HTTP 413 and preserves the existing target file

#### Scenario: Configuration upload remains within the limit
- **WHEN** an authenticated operator uploads a valid configuration at or below the configured import limit
- **THEN** the existing validation and secure-write behavior applies unchanged
