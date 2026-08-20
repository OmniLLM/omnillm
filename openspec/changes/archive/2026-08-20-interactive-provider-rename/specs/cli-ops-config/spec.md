## ADDED Requirements

### Requirement: Interactive provider rename

The CLI SHALL prompt for provider rename metadata when invoked interactively
without rename fields and MUST retain non-interactive flag validation for
automation.

#### Scenario: Interactive provider rename without metadata flags

- **WHEN** an operator invokes `omnillm provider rename` from an interactive
  terminal, selects or supplies a provider, and supplies neither `--name` nor
  `--alias`
- **THEN** the CLI prompts for a new display name and alias and submits the
  non-empty values after at least one value is provided

#### Scenario: Interactive provider rename leaves both fields empty

- **WHEN** the interactive rename prompts produce neither a display name nor an
  alias
- **THEN** the CLI returns an actionable validation error without sending a
  rename request

#### Scenario: Flag-driven provider rename

- **WHEN** an operator supplies `--name`, `--alias`, or its compatible hidden
  spelling to `provider rename`
- **THEN** the CLI uses the supplied fields without prompting and preserves the
  existing conflict validation

#### Scenario: Non-interactive provider rename without metadata flags

- **WHEN** automation invokes `provider rename` without `--name` or `--alias`
- **THEN** the CLI fails validation without prompting or sending a rename
  request
