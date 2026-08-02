# routing-failover Specification

## Purpose
Defines how model names become ordered provider attempts and how virtual models, balancing, affinity, candidate isolation, failover, and terminal errors control dispatch.
## Requirements
### Requirement: Provider-prefixed model resolution
A model name in `<provider>/<model>` form SHALL pin dispatch to the provider instance resolved first by instance identifier and then by provider subtitle.

#### Scenario: Prefix resolves
- **WHEN** the prefix identifies a provider instance
- **THEN** exactly one attempt targets that provider with the bare model name

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

