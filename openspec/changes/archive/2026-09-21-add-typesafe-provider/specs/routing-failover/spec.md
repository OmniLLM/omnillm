## ADDED Requirements

### Requirement: Evaluation capability routing
System One requests SHALL resolve only active TypeSafe instances using existing
model enablement, provider identity resolution, priority, virtual model ordering,
and explicit pinning rules. Ordinary discovery SHALL resolve advertised aliases.
An explicitly provider-qualified versioned identifier MAY reach that TypeSafe
instance even when omitted from its catalog, subject to existing explicit-routing
policy. Provider qualification SHALL remove only the recognized qualifier.

#### Scenario: Alias evaluation
- **WHEN** an unqualified `jev-latest` request matches multiple eligible TypeSafe instances
- **THEN** evaluation candidates follow existing provider ordering

#### Scenario: Version-pinned evaluation
- **WHEN** a request explicitly selects a TypeSafe instance and `jev-1.13.0`
- **THEN** that unchanged version identifier reaches only the selected instance even if discovery lists aliases alone

#### Scenario: Disabled alias
- **WHEN** a TypeSafe alias is disabled for ordinary routing
- **THEN** an unqualified request does not select that disabled model

#### Scenario: Evaluation failover
- **WHEN** an unpinned evaluation has multiple eligible candidates and the first fails with an existing retryable failure classification
- **THEN** dispatch may continue to the next eligible TypeSafe candidate while terminal cancellation stops dispatch

### Requirement: Evaluation and generation isolation
Generation dispatch SHALL exclude evaluation-only providers before upstream
execution. Explicitly selecting a TypeSafe provider for Chat Completions, Messages,
Responses, or embeddings SHALL produce a client-visible unsupported-capability
error without an upstream request. System One dispatch MUST exclude providers
that do not implement native evaluation.

#### Scenario: TypeSafe selected for chat
- **WHEN** a Chat Completions, Messages, Responses, or embeddings request explicitly selects TypeSafe
- **THEN** the gateway returns an unsupported-capability error without contacting TypeSafe

#### Scenario: Virtual model contains incompatible candidates
- **WHEN** a virtual model contains both evaluation-only and generation providers
- **THEN** each endpoint considers only candidates that support its operation and preserves the remaining candidates' relative order
