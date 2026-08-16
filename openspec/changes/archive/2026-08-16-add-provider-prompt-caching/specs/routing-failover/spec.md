## ADDED Requirements

### Requirement: Cacheable-prefix channel affinity
When a request contains prompt-cache directives, channel affinity SHALL derive a non-reversible key from the ordered provider-rendered prefix through the last effective cache boundary, including model, selected cache mode, ordered tools and schemas, system blocks, cache-control values, and configured user identity.

#### Scenario: Suffix varies after breakpoint
- **WHEN** two requests have byte-equivalent canonical prefixes through the same final breakpoint and differ only after that breakpoint
- **THEN** they resolve to the same affinity key

#### Scenario: Prefix changes before breakpoint
- **WHEN** content, tool order, cache TTL, model, provider mode, or native cache key changes within the effective cached prefix
- **THEN** the request resolves to a different affinity key

### Requirement: Prompt-cache affinity across virtual and instance routing
An unexpired prompt-cache affinity record SHALL prefer the previously successful eligible virtual upstream and provider instance while preserving every remaining candidate and its relative fallback order.

#### Scenario: Virtual upstream affinity hit
- **WHEN** a virtual model's cached prefix was successfully served by a still-eligible non-leading upstream and provider instance
- **THEN** that upstream and instance are tried first and all alternatives remain available for failover

#### Scenario: Preferred provider fails
- **WHEN** the affinity-preferred provider returns a retryable failure
- **THEN** dispatch continues through the unchanged remaining candidates

### Requirement: Successful prompt-cache affinity recording
Prompt-cache affinity SHALL be recorded only for a successful non-streaming execution or a streaming execution that begins successfully, and SHALL NOT be recorded for failed, canceled, or pre-response attempts.

#### Scenario: Canceled stream
- **WHEN** an upstream stream is canceled before successful response start
- **THEN** no prompt-cache affinity record is written for that candidate

### Requirement: Independent affinity lifetime
Prompt-cache affinity expiry SHALL be configured independently of provider cache TTL and SHALL default to five minutes.

#### Scenario: One-hour provider control
- **WHEN** a request uses a one-hour provider cache TTL
- **THEN** the affinity record still uses its configured lifetime rather than silently extending to one hour

## MODIFIED Requirements

### Requirement: Channel affinity
The gateway SHALL move the provider that previously served a stable legacy conversation head or explicit cacheable prefix to the front of candidates without removing failover alternatives.

#### Scenario: Affinity hit
- **WHEN** an unexpired pinned provider exists among multiple candidates
- **THEN** it moves to the front and all other relative ordering is preserved

#### Scenario: Affinity provider fails
- **WHEN** the preferred provider returns a retryable error
- **THEN** dispatch continues through the remaining candidates

#### Scenario: Unmarked legacy request
- **WHEN** a request contains no prompt-cache directive
- **THEN** existing stable-head affinity behavior remains available without requiring cache metadata
