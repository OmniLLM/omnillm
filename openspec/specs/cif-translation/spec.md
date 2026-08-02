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
Translation SHALL preserve tool-call identifiers, names, arguments, and result relationships across response emission and later multi-turn ingestion.

#### Scenario: Streamed arguments
- **WHEN** tool arguments arrive as partial JSON deltas
- **THEN** deltas accumulate in order without repeated block announcements resetting prior data

#### Scenario: Interleaved tool calls
- **WHEN** multiple tool calls are interleaved or reuse a provider block index across content kinds
- **THEN** distinct calls remain independently accumulated and preserve emission order

