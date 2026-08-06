# routing-failover Specification

## Purpose
Defines how model names become ordered provider attempts and how virtual models, balancing, affinity, candidate isolation, failover, and terminal errors control dispatch.
## Requirements
### Requirement: Provider-prefixed model resolution
A slash-containing model name SHALL first undergo ordinary resolution as a complete native model identifier. When an active provider advertises that complete identifier, ordinary provider ordering and failover SHALL take precedence even if the first segment also identifies a provider instance or subtitle. Only when the complete identifier is unavailable SHALL a first segment that resolves to a provider instance identifier or provider subtitle pin dispatch to that provider and remove only that recognized qualifier.

#### Scenario: Advertised native identifier collides with provider prefix
- **WHEN** an active provider advertises the complete slash-containing model identifier and its first segment also identifies a provider instance or subtitle
- **THEN** the complete model identifier is resolved through ordinary provider selection and retains normal provider ordering and failover

#### Scenario: Instance qualifier fallback
- **WHEN** the complete slash-containing identifier is unavailable and the first segment identifies a provider instance
- **THEN** exactly one fallback attempt targets that provider with only the recognized qualifier removed from the model name

#### Scenario: Subtitle qualifier fallback
- **WHEN** the complete slash-containing identifier is unavailable and the first segment identifies a provider subtitle
- **THEN** exactly one fallback attempt targets the resolved provider instance with only the recognized qualifier removed from the model name

#### Scenario: Native namespaced model
- **WHEN** a slash-containing model name is advertised by an active provider and its first segment does not identify a provider instance or subtitle
- **THEN** the complete model name is resolved through ordinary provider selection without removing a segment

#### Scenario: Explicit qualifier before namespaced model
- **WHEN** the complete qualified string is unavailable and a recognized provider qualifier precedes a native model identifier that itself contains one or more slashes
- **THEN** fallback dispatch pins the resolved provider and preserves every slash and segment in the native model identifier

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
The gateway SHALL move the provider that previously served a stable conversation head to the front of candidates without removing failover alternatives.

#### Scenario: Affinity hit
- **WHEN** an unexpired pinned provider exists among multiple candidates
- **THEN** it moves to the front and all other relative ordering is preserved

#### Scenario: Affinity provider fails
- **WHEN** the preferred provider returns a retryable error
- **THEN** dispatch continues through the remaining candidates

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

