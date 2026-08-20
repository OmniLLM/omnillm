## MODIFIED Requirements

### Requirement: Tool-call fidelity
Translation SHALL preserve tool-call identifiers, names, required arguments, non-empty optional arguments, original supported result values, and result relationships across response emission and later multi-turn ingestion. Before client-facing emission, translation SHALL omit an object property whose value is an empty string only when the selected tool's declared input schema identifies that property as optional; translation SHALL otherwise preserve the model-emitted arguments unchanged except for an explicitly selected client-compatibility policy whose exact argument repair and schema authorization are defined by the client-facing gateway contract.

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

#### Scenario: Selected client compatibility policy
- **WHEN** a client-facing gateway contract explicitly selects a compatibility repair and the selected tool schema authorizes its replacement value
- **THEN** translation applies only that exact repair and preserves all other model-emitted arguments unchanged
