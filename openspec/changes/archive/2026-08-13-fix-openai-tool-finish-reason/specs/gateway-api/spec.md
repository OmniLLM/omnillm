## ADDED Requirements

### Requirement: OpenAI tool-call finish reason
An OpenAI-compatible non-streaming response containing one or more assistant tool calls SHALL report `finish_reason` as `tool_calls`, even when upstream terminal metadata reports an ordinary stop.

#### Scenario: Canonical tool use has inconsistent upstream stop
- **WHEN** a provider response contains canonical tool calls but carries an upstream end-turn or stop reason
- **THEN** the OpenAI-compatible choice reports `finish_reason: tool_calls` and preserves the tool calls for replay

#### Scenario: Ordinary terminal response
- **WHEN** a provider response contains no tool calls and terminates normally
- **THEN** the OpenAI-compatible choice retains the ordinary `stop` finish reason
