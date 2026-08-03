## ADDED Requirements

### Requirement: Non-mutating diagnostic truncation
Truncating a request or response payload for diagnostic logging SHALL NOT modify or alias the caller's buffer. The truncated value SHALL be produced in newly allocated storage so that the payload subsequently transmitted upstream is byte-identical to the payload that was marshalled.

#### Scenario: Oversized payload is trace-logged
- **WHEN** a payload larger than the trace body limit is truncated for logging and the same buffer is then sent upstream
- **THEN** the transmitted bytes are unchanged and contain no truncation marker

#### Scenario: Payload within limit
- **WHEN** a payload at or below the trace body limit is truncated for logging
- **THEN** the value is returned unchanged and the caller's buffer is untouched

#### Scenario: Repeated truncation
- **WHEN** the same buffer is truncated for logging more than once
- **THEN** each result is identical and the source buffer remains unmodified
