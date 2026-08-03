## ADDED Requirements

### Requirement: Encoding-safe log previews
Values truncated for inclusion in structured log fields SHALL be cut at a UTF-8 character boundary at or below the configured byte limit, so a truncated preview is always valid UTF-8. Truncated values SHALL retain their existing truncation marker, and values within the limit SHALL be emitted unchanged.

#### Scenario: Multi-byte character at the boundary
- **WHEN** a tool-loop preview is truncated at a byte offset that falls inside a multi-byte character
- **THEN** the preview is cut before that character and the emitted log field is valid UTF-8

#### Scenario: ASCII value truncated
- **WHEN** an ASCII value longer than the limit is truncated
- **THEN** it is cut at the limit and carries the truncation marker

#### Scenario: Value within limit
- **WHEN** a value at or below the limit is prepared for logging
- **THEN** it is emitted unchanged with no truncation marker
