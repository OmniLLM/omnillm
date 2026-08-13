## ADDED Requirements

### Requirement: Copilot Responses custom-tool fidelity
The Copilot Responses adapter SHALL preserve native custom tool definitions, call history, results, provider output, raw input streaming, names, ordering, and call identifiers without coercing custom tools to function calls.

#### Scenario: Custom definition and history request
- **WHEN** a canonical request contains a custom tool or prior custom call/result
- **THEN** Copilot receives native custom tool definitions and custom call/output items with the preserved format, raw values, name, and `call_id`

#### Scenario: Non-streaming provider custom call
- **WHEN** Copilot returns a `custom_tool_call`
- **THEN** canonical output preserves the custom kind, name, identifier, namespace, exact raw input, and tool-use stop reason

#### Scenario: Streaming provider custom call
- **WHEN** Copilot streams custom output items and interleaved custom-input deltas
- **THEN** canonical stream events retain each call's index, identity, order, and raw input without duplicate done content or cross-association

#### Scenario: Non-Responses provider fallback
- **WHEN** a custom tool request is handled by a provider path without native custom-tool support
- **THEN** the adapter may use the retained function-compatible schema, arguments, and result text without changing existing function-tool behavior
