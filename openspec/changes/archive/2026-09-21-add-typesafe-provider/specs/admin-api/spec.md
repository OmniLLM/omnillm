## ADDED Requirements

### Requirement: TypeSafe administration lifecycle
Authenticated provider setup, re-authentication, listing, metadata changes,
activation, deactivation, model refresh, and deletion SHALL support TypeSafe
instances using existing provider identity and credential persistence contracts.
Public provider responses MUST NOT expose the API key.

#### Scenario: Create and manage TypeSafe
- **WHEN** an authenticated operator creates a TypeSafe instance with a valid key
- **THEN** the returned instance can be listed, renamed, refreshed, activated, deactivated, and deleted through ordinary lifecycle operations

#### Scenario: Re-authentication failure
- **WHEN** a replacement TypeSafe key fails authentication
- **THEN** the existing provider's durable identity, metadata, and working credential remain intact

### Requirement: System One metering
Completed System One evaluations SHALL appear in the existing metering views
with API shape `systemone`, selected provider instance, model, and upstream input
and output token counts. Provider prompt-cache details SHALL remain unknown when
not supplied upstream, and the request SHALL NOT be recorded as an exact-response
cache hit.

#### Scenario: Evaluation usage
- **WHEN** TypeSafe returns input_tokens and output_tokens for a successful evaluation
- **THEN** metering records those counts once under the System One API shape without storing state, question content, or credentials
