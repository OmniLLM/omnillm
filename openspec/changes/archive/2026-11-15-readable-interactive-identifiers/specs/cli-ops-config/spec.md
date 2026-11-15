## ADDED Requirements

### Requirement: Readable interactive entity selection

When an interactive CLI command presents a list of existing entities identified
by IDs, the CLI SHALL display each full ID together with available human-readable
identity metadata. Provider choices SHALL include the display name and alias when
available, using the legacy subtitle as the alias fallback. Virtual-model choices
SHALL include the display name when available. Search SHALL match the displayed
identity fields case-insensitively. Selection MUST resolve to the original
canonical ID, not to the rendered label, name, or alias.

#### Scenario: Choose a provider instance
- **WHEN** an operator invokes `provider show`, `provider enable`, or another
  command with an interactive provider-instance picker and several providers exist
- **THEN** each choice shows its full ID and available display name and alias,
  and selecting a choice uses that provider's original ID

#### Scenario: Choose a virtual model
- **WHEN** an operator invokes `virtualmodel get`, `update`, or `delete` with an
  interactive virtual-model picker and several models exist
- **THEN** each choice shows its full ID and available display name, and selecting
  a choice uses that virtual model's original ID

#### Scenario: Search identity fields
- **WHEN** an operator searches an interactive entity picker by a case-insensitive
  substring of an ID, displayed name, or displayed alias
- **THEN** matching choices remain selectable and retain their original IDs

#### Scenario: Duplicate or missing descriptive metadata
- **WHEN** entities have the same display name or lack a name or alias
- **THEN** their full IDs remain visible and distinguishable, and a choice with no
  descriptive metadata remains selectable by ID

#### Scenario: Legacy provider alias
- **WHEN** a provider response has no non-empty alias but has a legacy subtitle
- **THEN** its interactive choice displays and searches that subtitle as its alias

#### Scenario: Explicit arguments and automation
- **WHEN** a command receives an explicit entity reference or is invoked without
  a reference in a non-interactive context
- **THEN** existing argument resolution and non-interactive validation are
  preserved without introducing a prompt

#### Scenario: Empty, single, and cancelled selections
- **WHEN** an entity list is empty, contains one valid entry, or selection is
  cancelled
- **THEN** the CLI preserves the existing error, automatic single-item selection,
  or cancellation behavior respectively

#### Scenario: Untrusted metadata
- **WHEN** entity metadata contains control characters or unrelated fields such
  as credentials
- **THEN** rendered choices cannot execute terminal control sequences or expose
  unrelated fields, and selected IDs are not altered
