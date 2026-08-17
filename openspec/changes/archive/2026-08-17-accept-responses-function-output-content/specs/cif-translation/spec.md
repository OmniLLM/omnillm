## MODIFIED Requirements

### Requirement: Canonical content vocabulary
CIF SHALL represent ordered message content as typed text, image, thinking, tool-call, and tool-result parts, SHALL reject unknown part types during serialization, and SHALL retain an optional original structured output value for ordinary and custom tool results while providing serialized text for provider fallbacks.

#### Scenario: Tool result conversion
- **WHEN** a tool result enters canonical form
- **THEN** its call identifier, tool name, content, and error state are preserved

#### Scenario: Structured function result conversion
- **WHEN** an ordinary Responses function result contains an ordered list of supported text, image, or file content items
- **THEN** canonical form preserves the original ordered list and a compact JSON textual fallback with the same semantic content

#### Scenario: Thinking signature
- **WHEN** reasoning includes a provider signature
- **THEN** canonical thinking retains the signature for compatible replay

### Requirement: Tool-call fidelity
Translation SHALL preserve tool-call identifiers, names, required arguments, non-empty optional arguments, original supported result values, and result relationships across response emission and later multi-turn ingestion. Before client-facing emission, translation SHALL omit an object property whose value is an empty string only when the selected tool's declared input schema identifies that property as optional; translation SHALL otherwise preserve the model-emitted arguments unchanged.

#### Scenario: Streamed arguments
- **WHEN** tool arguments arrive as partial JSON deltas
- **THEN** deltas accumulate in order without repeated block announcements resetting prior data

#### Scenario: Optional empty string
- **WHEN** a completed tool call contains an empty-string object property that is not listed as required by the selected tool's declared input schema
- **THEN** translation omits that property from the emitted tool arguments

#### Scenario: Required empty string
- **WHEN** a completed tool call contains an empty-string object property that is listed as required by the selected tool's declared input schema
- **THEN** translation preserves that property and its empty-string value

#### Scenario: Arguments without a usable schema
- **WHEN** the selected tool has no usable declared input schema for an emitted argument property
- **THEN** translation preserves the model-emitted argument property unchanged

#### Scenario: Interleaved tool calls
- **WHEN** multiple tool calls are interleaved or reuse a provider block index across content kinds
- **THEN** distinct calls remain independently accumulated, normalized against their own tool schemas, and emitted in their original order

#### Scenario: Structured function output replay
- **WHEN** a supported structured function result is replayed through a native Responses provider path
- **THEN** translation emits the original ordered output value with the same call relationship instead of replacing it with fallback text

#### Scenario: Structured function output fallback
- **WHEN** a supported structured function result is translated to a provider path that only accepts textual tool results
- **THEN** translation emits its compact JSON fallback text without losing or reordering content members

#### Scenario: Cached tool-call replay
- **WHEN** a normalized streamed tool call is stored and replayed from the response cache
- **THEN** replay emits the same identifiers, ordering, and normalized arguments as the original response

## ADDED Requirements

### Requirement: Responses function-output normalization
Responses ingestion SHALL accept `function_call_output.output` as either a string or an ordered list containing only `input_text`, `input_image`, or `input_file` content items valid under the maintained OpenAI Responses schema. A string output SHALL remain unchanged. A content-list output SHALL retain its complete original ordered value and SHALL have a compact JSON textual fallback. Ingestion MUST reject absent outputs, unsupported item types, malformed item objects, and invalid supplied field values without partially translating the request.

#### Scenario: String function output
- **WHEN** a `function_call_output` contains a string output
- **THEN** canonical translation preserves the string unchanged and associates it with the same `call_id`

#### Scenario: Ordered text output
- **WHEN** a `function_call_output` contains multiple valid `input_text` items
- **THEN** canonical translation preserves every item and its order and produces compact JSON fallback text

#### Scenario: Ordered multimodal output
- **WHEN** a `function_call_output` contains a valid ordered mixture of text, image, and file content items
- **THEN** canonical translation preserves every supported field, item order, and call relationship

#### Scenario: Explicit empty values
- **WHEN** a `function_call_output` contains an explicitly present empty string or empty content list
- **THEN** canonical translation accepts it and distinguishes it from an absent output

#### Scenario: Type-only and nullable multimodal output
- **WHEN** a `function_call_output` contains schema-valid `input_image` or `input_file` members with no optional fields or explicitly null optional fields
- **THEN** canonical translation preserves every member, null, field, item order, and call relationship

#### Scenario: Invalid structured output
- **WHEN** a `function_call_output` content list contains an unsupported type, non-object member, missing required text, or invalid supplied image/file field value
- **THEN** ingestion rejects the request with an invalid-request error that identifies the malformed function output
