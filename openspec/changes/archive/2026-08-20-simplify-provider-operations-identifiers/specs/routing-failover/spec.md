## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: Provider-qualified model aliases

**Reason**: Alias behavior is generalized into Provider-prefixed model
resolution, alongside provider IDs and display names.

**Migration**: Existing persisted subtitle aliases and `alias/model` strings
continue to resolve under the generalized requirement.

#### Scenario: Existing alias migrates to generalized resolution

- **WHEN** a client uses an existing persisted subtitle as the qualifier in
  `alias/model`
- **THEN** the generalized provider-prefixed resolver treats it as the provider
  alias and preserves the previous routing behavior
