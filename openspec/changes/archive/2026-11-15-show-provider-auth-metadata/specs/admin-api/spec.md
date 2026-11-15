## ADDED Requirements

### Requirement: Provider authentication metadata inspection
The authenticated administration API SHALL provide a read-only provider-scoped metadata operation accepting the same instance ID, alias, and display-name references as other provider operations. It SHALL return only explicitly allowlisted, available non-secret metadata, including a stable authorization endpoint and OAuth client ID where applicable, without starting an authorization flow or modifying provider state. Unknown or unavailable values SHALL be omitted rather than inferred.

#### Scenario: Inspect configured OAuth provider
- **WHEN** an authenticated operator requests metadata for a supported OAuth provider with an available client ID
- **THEN** the operation returns its canonical provider identity, stable authorization endpoint, and client ID without disclosing credential material

#### Scenario: Inspect API-key provider
- **WHEN** an authenticated operator requests metadata for a provider without applicable OAuth fields
- **THEN** the operation returns only applicable allowlisted metadata and does not fabricate an OAuth client ID or authorization URL

#### Scenario: Unauthenticated metadata request
- **WHEN** an unauthenticated caller requests provider metadata
- **THEN** the operation is rejected under the existing admin authentication policy

#### Scenario: Ambiguous provider reference
- **WHEN** a metadata request uses an ambiguous provider reference
- **THEN** the operation returns the existing actionable ambiguity error rather than choosing an instance
