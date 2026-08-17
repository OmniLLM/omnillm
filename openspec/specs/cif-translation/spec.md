# cif-translation Specification

## Purpose
Defines the Canonical Intermediate Format that separates inbound client dialects from provider execution and outbound serialization for requests, responses, and streams.
## Requirements
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

### Requirement: Canonical request
A canonical request SHALL preserve the caller's original model, ordered messages, tools, tool choice, optional sampling values, stop sequences, structured output format, streaming intent, and typed extensions.

#### Scenario: Optional zero value
- **WHEN** temperature, top-p, or max tokens is omitted
- **THEN** CIF distinguishes omission from an explicitly supplied zero

#### Scenario: Single stop string
- **WHEN** an inbound dialect supplies a bare stop string
- **THEN** CIF normalizes it to a one-element sequence list

### Requirement: Canonical response
A canonical response SHALL preserve provider identity, provider-reported model, ordered generated content, normalized stop reason, matched stop sequence, and optional usage including cache-read and cache-write tokens.

#### Scenario: Stop normalization
- **WHEN** a provider reports its terminal reason
- **THEN** CIF maps it to the supported canonical stop-reason vocabulary

### Requirement: Canonical stream events
CIF SHALL model streams as ordered start, indexed content delta, content stop, end, and error events with sufficient identity and type information for every output dialect.

#### Scenario: New content block
- **WHEN** a provider opens a tool or text block
- **THEN** CIF announces the block shape and index before subsequent deltas

#### Scenario: Stream end
- **WHEN** generation completes
- **THEN** the end event carries normalized stop information and usage when known

### Requirement: Cross-dialect serialization
Canonical responses and streams SHALL be serializable independently of the inbound dialect so one canonical result can satisfy any supported client shape.

#### Scenario: Cross-dialect response
- **WHEN** a result ingested from an OpenAI-shaped flow is serialized for Anthropic
- **THEN** serialization succeeds without relying on retained OpenAI wire state

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

### Requirement: Tool-use semantic normalization
Canonical response normalization SHALL treat the presence of valid tool-call content as tool use when provider stop metadata is less specific, so downstream serializers expose a consistent tool-loop contract.

#### Scenario: Tool call with generic terminal metadata
- **WHEN** a canonical response contains at least one valid tool-call part and its stop reason is end-turn or stop
- **THEN** downstream serialization treats the response as tool use

#### Scenario: Tool result continuation
- **WHEN** a client replays the assistant tool call and corresponding tool result in a subsequent request
- **THEN** the canonical conversation remains valid and can produce a terminal assistant response

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

### Requirement: Native Responses custom-tool representation
CIF SHALL preserve the native Responses custom-tool discriminator, custom definition format, raw call input, optional namespace, and original output value while retaining function-compatible argument and text fallbacks. Existing CIF values that omit the discriminator SHALL continue to represent function tools.

#### Scenario: Custom tool definition
- **WHEN** a Responses request declares a custom tool with a format
- **THEN** CIF preserves its custom kind and format while retaining a fallback schema with one required string `input`

#### Scenario: Raw custom call input
- **WHEN** a custom call contains arbitrary text, including an explicitly empty string
- **THEN** CIF preserves the exact raw input and custom kind without JSON parsing while retaining the same text under fallback argument `input`

#### Scenario: Original custom output
- **WHEN** a custom output is a string or supported ordered content list
- **THEN** CIF preserves the original value and custom kind while retaining normalized text content for non-Responses fallbacks

#### Scenario: Legacy function tool
- **WHEN** an existing CIF tool, call, or result omits the new kind field
- **THEN** all serializers and providers continue treating it as an ordinary function tool

### Requirement: Responses additional-tool name fidelity
Responses ingestion SHALL preserve the declared name of each nested `additional_tools` definition as the canonical callable tool name. Transport namespace labels SHALL NOT be prepended to the declared name unless a reversible client-facing mapping preserves the original name through the complete call round trip.

#### Scenario: Codex functions namespace
- **WHEN** Codex declares a nested custom tool named `exec` under an `additional_tools` namespace named `functions`
- **THEN** canonical translation exposes the callable name as `exec` and not `functions__exec`

#### Scenario: Duplicate declared names
- **WHEN** multiple nested namespace groups declare the same callable tool name
- **THEN** translation retains one deterministic canonical definition for that name rather than inventing namespace-prefixed names the client did not register

### Requirement: Canonical cache-control placement
CIF SHALL preserve optional cache control on ordered system blocks, every supported cacheable message content part, and tool definitions, including text, image, thinking, tool-call, and tool-result parts.

#### Scenario: Marked multi-part message
- **WHEN** a message contains marked and unmarked content parts in a defined order
- **THEN** CIF preserves every part, its cache-control association, and its original relative order

### Requirement: Structured canonical system content
CIF SHALL represent top-level system content as ordered blocks and SHALL retain mid-conversation system messages in their original conversation position.

#### Scenario: Structured top-level system
- **WHEN** an inbound request contains multiple system text blocks with a breakpoint on one block
- **THEN** canonical form preserves both block boundaries, text, order, and the marked block

#### Scenario: Mid-conversation system instruction
- **WHEN** a supported inbound request places a system instruction after prior conversation turns
- **THEN** canonical form retains that message at its original position instead of hoisting it into the top-level system content

### Requirement: Canonical request cache policy
A canonical request SHALL separately retain top-level automatic cache control, native prompt cache key, and native prompt cache retention, and legacy requests that omit these fields SHALL remain valid.

#### Scenario: Legacy request round trip
- **WHEN** a canonical request without cache metadata is serialized, copied for candidates, and deserialized
- **THEN** it retains its existing semantic fields and does not acquire cache directives

### Requirement: Canonical cache usage totals
CIF SHALL define total prompt input independently from optional uncached, cache-read, cache-write, five-minute-write, and one-hour-write detail and SHALL reject or ignore negative and internally inconsistent provider counters without producing invalid totals.

#### Scenario: Detailed write split
- **WHEN** a provider reports total cache writes together with five-minute and one-hour components
- **THEN** CIF preserves the components only when their sum does not exceed the total cache writes

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
