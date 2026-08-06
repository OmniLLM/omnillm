## MODIFIED Requirements

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
