## ADDED Requirements

### Requirement: Native Responses custom-tool output
The public Responses API SHALL serialize canonical custom tool calls as native `custom_tool_call` items with their original declared name and raw input.

#### Scenario: Non-streaming custom call
- **WHEN** a canonical response contains a custom tool call
- **THEN** the Responses output contains `type: custom_tool_call`, the original `call_id`, name, raw `input`, and no function `arguments`

#### Scenario: Streaming custom input
- **WHEN** a provider streams a custom tool call and raw input
- **THEN** the Responses stream emits a custom output-item announcement followed by `response.custom_tool_call_input.delta`, `response.custom_tool_call_input.done`, output-item completion, and final response completion in order

#### Scenario: Function call remains unchanged
- **WHEN** a canonical response contains an ordinary function call
- **THEN** Responses serialization continues to emit existing `function_call` items and function-argument events
