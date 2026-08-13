# cif-translation Specification

## Purpose
Defines the Canonical Intermediate Format that separates inbound client dialects from provider execution and outbound serialization for requests, responses, and streams.
## Requirements
### Requirement: Canonical content vocabulary
CIF SHALL represent ordered message content as typed text, image, thinking, tool-call, and tool-result parts and SHALL reject unknown part types during serialization.

#### Scenario: Tool result conversion
- **WHEN** a tool result enters canonical form
- **THEN** its call identifier, tool name, content, and error state are preserved

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
Translation SHALL preserve tool-call identifiers, names, required arguments, non-empty optional arguments, and result relationships across response emission and later multi-turn ingestion. Before client-facing emission, translation SHALL omit an object property whose value is an empty string only when the selected tool's declared input schema identifies that property as optional; translation SHALL otherwise preserve the model-emitted arguments unchanged.

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

