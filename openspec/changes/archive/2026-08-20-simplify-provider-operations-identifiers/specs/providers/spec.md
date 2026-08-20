## MODIFIED Requirements

### Requirement: Provider instance registration

Every provider SHALL have a unique canonical instance identifier independent of
provider type, a non-empty display name, and an optional short alias. Registering
the same identifier SHALL replace its previous in-process registration. Provider
references MUST resolve in deterministic order by exact instance ID, unique
case-insensitive alias, then unique case-insensitive display name.

#### Scenario: Resolve exact instance ID

- **WHEN** a reference exactly equals a registered provider instance ID
- **THEN** that instance is returned without consulting alias or name matches

#### Scenario: Resolve unique alias

- **WHEN** no exact ID matches and exactly one provider alias matches the
  reference case-insensitively
- **THEN** that provider's canonical instance ID is returned

#### Scenario: Resolve unique display name

- **WHEN** no ID or alias matches and exactly one provider display name matches
  the reference case-insensitively
- **THEN** that provider's canonical instance ID is returned

#### Scenario: Ambiguous alias or display name

- **WHEN** the highest-precedence matching alias or display name belongs to more
  than one provider instance
- **THEN** resolution fails without selecting an instance and identifies the
  matching canonical instance IDs

#### Scenario: Unknown instance

- **WHEN** no instance ID, alias, or display name matches a provider reference
- **THEN** lookup fails without selecting an instance

#### Scenario: Legacy subtitle persistence

- **WHEN** an existing provider has a persisted `subtitle`
- **THEN** that value is treated as its alias without rewriting its instance ID,
  credentials, or owned records
