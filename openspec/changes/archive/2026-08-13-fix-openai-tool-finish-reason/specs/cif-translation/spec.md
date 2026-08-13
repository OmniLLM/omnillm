## ADDED Requirements

### Requirement: Tool-use semantic normalization
Canonical response normalization SHALL treat the presence of valid tool-call content as tool use when provider stop metadata is less specific, so downstream serializers expose a consistent tool-loop contract.

#### Scenario: Tool call with generic terminal metadata
- **WHEN** a canonical response contains at least one valid tool-call part and its stop reason is end-turn or stop
- **THEN** downstream serialization treats the response as tool use

#### Scenario: Tool result continuation
- **WHEN** a client replays the assistant tool call and corresponding tool result in a subsequent request
- **THEN** the canonical conversation remains valid and can produce a terminal assistant response
