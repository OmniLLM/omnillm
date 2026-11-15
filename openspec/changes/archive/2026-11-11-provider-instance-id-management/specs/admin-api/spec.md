## ADDED Requirements

### Requirement: Provider instance ID administration
The authenticated administration API SHALL expose an operation to change a provider's canonical instance ID, returning its new ID on success. A changed ID MUST be 1 to 64 ASCII characters, begin with a letter or digit, and contain only letters, digits, periods, underscores, and hyphens. The API SHALL reject invalid or conflicting IDs without modifying either provider and SHALL preserve existing metadata-update contracts. An unchanged legacy ID SHALL remain a valid no-op even if it does not meet the new-ID rules.

#### Scenario: Change ID by provider reference
- **WHEN** an authenticated operator supplies a uniquely resolvable existing provider reference and a valid unused new ID
- **THEN** the operation returns the new canonical ID and subsequent provider-scoped operations accept it

#### Scenario: Invalid or conflicting target
- **WHEN** the requested ID is invalid or conflicts with another provider ID, alias, or display name under provider-reference resolution
- **THEN** the operation returns an actionable client error and neither instance changes

#### Scenario: Same ID
- **WHEN** the requested ID exactly matches the current canonical ID
- **THEN** the operation succeeds as a no-op without changing provider state

#### Scenario: Unauthenticated change
- **WHEN** a caller without valid administrative credentials requests an ID change
- **THEN** the operation is rejected without modifying state
