## ADDED Requirements

### Requirement: Cached native tool-call fidelity
Canonical response caching SHALL preserve a tool call's identifier, declared name, kind, function-compatible arguments, exact optional raw custom input including an explicitly present empty value, optional namespace, content order, and normalized stop semantics. Cache entries written before these optional fields existed SHALL remain readable as ordinary function calls.

#### Scenario: Native custom call round trip
- **WHEN** a canonical custom tool call with raw input and namespace is stored and decoded
- **THEN** the decoded call retains its custom kind, identifier, name, exact raw input, namespace, fallback arguments, and relative order

#### Scenario: Explicit empty custom input round trip
- **WHEN** a custom tool call contains an explicitly present empty raw input
- **THEN** cache encode and decode preserve the present empty value rather than treating it as absent

#### Scenario: Legacy function cache entry
- **WHEN** a cached tool-call record omits kind, raw input, and namespace
- **THEN** decoding treats it as an ordinary function call with its existing identifier, name, and arguments

### Requirement: Cached native tool stream fidelity
Canonical stream accumulation and synthesis SHALL retain native custom tool metadata and accumulate custom raw-input deltas independently from function JSON-argument deltas, while preserving call order, block identity, and complete-stream eligibility.

#### Scenario: Streamed custom input accumulation
- **WHEN** a provider announces a custom call and emits multiple custom-input deltas
- **THEN** accumulation concatenates the deltas exactly and reconstructs one custom canonical call with the announced identity and namespace

#### Scenario: Synthetic custom replay
- **WHEN** a cached custom canonical call is synthesized for streaming replay
- **THEN** synthesis announces one custom call, emits custom-input deltas rather than function-argument deltas, closes that item once, and reaches one terminal event

#### Scenario: Interleaved function and custom calls
- **WHEN** function-argument and custom-input deltas are interleaved across multiple call identities or provider indexes
- **THEN** each call accumulates only its own delta vocabulary and all calls retain their original order

#### Scenario: Incomplete native tool stream
- **WHEN** a native tool stream errors or closes without a successful terminal event
- **THEN** no canonical response is available for cache population
