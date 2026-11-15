## ADDED Requirements

### Requirement: Provider enable and disable commands

The CLI SHALL advertise `omnillm provider enable` and `omnillm provider disable` as the canonical commands for adding and removing providers from the active set. It MUST NOT accept `provider activate` or `provider deactivate` as command aliases. The canonical commands SHALL preserve provider reference handling, validation, completion, output formats, and existing server-side activation semantics.

#### Scenario: Provider help

- **WHEN** an operator views ordinary provider command help
- **THEN** the CLI lists `enable` and `disable` as the active-set management commands rather than listing `activate` and `deactivate` as separate commands

#### Scenario: Enable a provider

- **WHEN** an operator invokes `provider enable` with a valid provider reference
- **THEN** the CLI requests the same active-set operation as the prior `provider activate` command

#### Scenario: Disable a provider

- **WHEN** an operator invokes `provider disable` with a valid provider reference
- **THEN** the CLI requests the same active-set operation as the prior `provider deactivate` command

#### Scenario: Legacy commands rejected

- **WHEN** an operator or automation invokes `provider activate` or `provider deactivate`
- **THEN** the CLI rejects the unknown command without requesting a provider state change
