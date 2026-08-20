# routing-failover Specification

## Purpose
Defines how model names become ordered provider attempts and how virtual models, balancing, affinity, candidate isolation, failover, and terminal errors control dispatch.
## Requirements
### Requirement: Provider-prefixed model resolution

A slash-containing model name SHALL first undergo ordinary resolution as a
complete native model identifier. When an active provider advertises that
complete identifier, ordinary provider ordering and failover SHALL take
precedence even if the first segment identifies a provider instance, alias, or
display name. Only when the complete identifier is unavailable SHALL a first
segment that uniquely resolves by exact instance ID, case-insensitive alias, or
case-insensitive display name pin dispatch to that provider and remove only that
recognized qualifier. An ambiguous qualifier MUST fail rather than select a
provider.

#### Scenario: Advertised native identifier collides with provider prefix

- **WHEN** an active provider advertises the complete slash-containing model
  identifier and its first segment also identifies a provider instance, alias,
  or display name
- **THEN** the complete model identifier is resolved through ordinary provider
  selection and retains normal provider ordering and failover

#### Scenario: Instance qualifier fallback

- **WHEN** the complete slash-containing identifier is unavailable and the first
  segment exactly identifies a provider instance
- **THEN** exactly one fallback attempt targets that provider with only the
  recognized qualifier removed from the model name

#### Scenario: Subtitle qualifier fallback

- **WHEN** the complete slash-containing identifier is unavailable and the first
  segment uniquely identifies a provider alias case-insensitively
- **THEN** exactly one fallback attempt targets the resolved provider instance
  with only the recognized qualifier removed from the model name

#### Scenario: Display-name qualifier fallback

- **WHEN** the complete slash-containing identifier is unavailable and the first
  segment uniquely identifies a provider display name case-insensitively
- **THEN** exactly one fallback attempt targets the resolved provider instance
  with only the recognized qualifier removed from the model name

#### Scenario: Ambiguous provider qualifier

- **WHEN** the complete slash-containing identifier is unavailable and the first
  segment ambiguously matches multiple aliases or display names
- **THEN** dispatch fails with an invalid provider-reference error identifying
  the matching instance IDs and no matched provider is called

#### Scenario: Native namespaced model

- **WHEN** a slash-containing model name is advertised by an active provider and
  its first segment does not uniquely identify a provider reference
- **THEN** the complete model name is resolved through ordinary provider
  selection without removing a segment

#### Scenario: Explicit qualifier before namespaced model

- **WHEN** the complete qualified string is unavailable and a recognized
  provider qualifier precedes a native model identifier that itself contains
  one or more slashes
- **THEN** fallback dispatch pins the resolved provider and preserves every
  slash and segment in the native model identifier

### Requirement: Virtual model expansion
An enabled virtual model SHALL expand into one attempt per configured upstream and SHALL fall back to direct resolution when disabled or unroutable.

#### Scenario: No routable upstream
- **WHEN** an enabled virtual model has no usable upstream
- **THEN** the gateway warns and produces a direct-model attempt rather than failing immediately

### Requirement: Virtual model ordering
The gateway SHALL support priority, round-robin, random, and weighted upstream ordering while retaining every upstream as a failover candidate.

#### Scenario: Round robin
- **WHEN** repeated requests use a round-robin virtual model
- **THEN** the leading upstream rotates per virtual model and the remaining order wraps

#### Scenario: Non-positive weight
- **WHEN** a weighted upstream has a weight below one
- **THEN** selection treats its weight as one

### Requirement: Candidate request isolation
Each provider candidate SHALL receive an independent canonical request copy and retain both canonical and provider-remapped model names.

#### Scenario: Provider remapping
- **WHEN** one adapter remaps its model name
- **THEN** no other candidate or the original request is mutated

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

### Requirement: Sequential failover
Dispatch SHALL try attempts and their candidates in order, return on first success, and otherwise return the most recent relevant error.

#### Scenario: Candidate succeeds
- **WHEN** a candidate completes successfully
- **THEN** no later candidate or attempt is called

#### Scenario: Attempt has no candidates
- **WHEN** resolution yields no candidates without an error
- **THEN** dispatch records a not-found error and continues to the next attempt

### Requirement: Terminal errors
Errors marked terminal SHALL stop failover immediately while preserving the wrapped error for classification.

#### Scenario: Client disconnect
- **WHEN** a candidate detects a disconnected client and returns a terminal cancellation
- **THEN** no remaining upstream request is issued

### Requirement: Provider-specific dispatch adjustments
Provider-specific retry and shape adjustments SHALL be applied only to the affected candidate.

#### Scenario: Copilot single-upstream mode
- **WHEN** a Copilot candidate is prepared
- **THEN** provider authentication retry and route streaming fallback are disabled for that candidate only

### Requirement: Correlatable provider failure diagnostics
When every candidate has failed and the gateway emits its terminal provider-failure error log, that log SHALL include the request identifier, the failing provider instance, and the upstream model, in addition to the underlying error. Client-disconnect cases SHALL retain their existing informational handling and SHALL NOT be logged as provider failures.

#### Scenario: All providers failed
- **WHEN** the gateway records a terminal provider failure after all candidates are exhausted
- **THEN** the error log carries the request identifier, provider instance, and upstream model so it can be correlated with the gateway request log

#### Scenario: Client canceled
- **WHEN** the terminal failure is a client disconnect
- **THEN** it remains an informational log and no provider-failure error is emitted

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
