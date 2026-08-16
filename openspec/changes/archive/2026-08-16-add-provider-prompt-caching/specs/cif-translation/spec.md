## ADDED Requirements

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
