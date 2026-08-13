## ADDED Requirements

### Requirement: Responses custom-tool normalization
Responses ingestion SHALL normalize explicitly typed custom tool calls and outputs into the canonical tool-call and tool-result vocabulary without parsing a custom call's raw input as JSON. The canonical call SHALL retain the `call_id`, tool name, and item order; its arguments SHALL contain exactly one `input` property whose string value equals the raw custom input. A string custom output SHALL remain unchanged, and a supported ordered content-list output SHALL be represented as compact JSON text without reordering its members. The custom wire discriminator and custom-only metadata are not required to survive this compatibility normalization.

#### Scenario: Raw multiline custom input
- **WHEN** a Responses history contains a `custom_tool_call` with a `call_id`, name, and multiline non-JSON `input`
- **THEN** canonical translation produces a tool call with the same identifier and name and an `input` argument equal to the original raw string

#### Scenario: Empty custom input
- **WHEN** a Responses custom call includes an explicitly present empty `input` string
- **THEN** canonical translation preserves the required `input` argument with an empty-string value

#### Scenario: String custom output
- **WHEN** a `custom_tool_call_output` contains a string output associated with a prior custom call
- **THEN** canonical translation preserves the string unchanged and associates the tool result with the same `call_id`

#### Scenario: Ordered content-list custom output
- **WHEN** a custom output contains an ordered list of supported text, image, or file content items
- **THEN** canonical translation preserves the complete ordered list as compact JSON tool-result content associated with the same `call_id`

#### Scenario: Mixed tool history
- **WHEN** assistant messages, function calls, and custom calls are interleaved before their results
- **THEN** canonical translation preserves assistant-part order and each call-to-result relationship

### Requirement: Responses custom-tool definition normalization
Responses ingestion SHALL normalize a named custom tool definition into a canonical tool definition whose input schema accepts exactly one required string property named `input`.

#### Scenario: Custom tool offered to a translated provider
- **WHEN** a Responses request declares a named custom tool
- **THEN** the canonical request contains the same name and description with a required string `input` schema suitable for existing function-tool provider adapters
