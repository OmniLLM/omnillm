## ADDED Requirements

### Requirement: Sensitive payload exclusion from logs
OmniLLM SHALL NOT emit tool argument content, tool result content, or credential material derived from those payloads into structured logs at any enabled log level or destination. This exclusion SHALL apply whether the payload is plain text, structured data, streamed in deltas, marked as an error, or longer than a diagnostic size limit.

#### Scenario: Secret-bearing tool arguments
- **WHEN** a tool call contains a synthetic credential sentinel in nested or streamed arguments
- **THEN** no structured log record contains the sentinel or any argument-content preview

#### Scenario: Secret-bearing tool result
- **WHEN** a client returns a plain-text or structured tool result containing a synthetic credential sentinel
- **THEN** no structured log record contains the sentinel or any result-content preview

#### Scenario: Errored Agent tool result at default level
- **WHEN** an Agent tool result is marked as an execution error and warning logs are enabled
- **THEN** the warning contains no tool result content or credential material
