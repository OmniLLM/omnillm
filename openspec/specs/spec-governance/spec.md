# spec-governance Specification

## Purpose
Defines the mandatory specification-driven development governance that keeps OmniLLM implementation, tests, and cumulative current-state specifications synchronized.
## Requirements
### Requirement: Specification artifacts precede implementation
Every behavior or code change MUST have an OpenSpec proposal, affected capability deltas, required design, and task checklist before implementation begins.

#### Scenario: Code change without artifacts
- **WHEN** a change modifies code, tests, dependencies, or build/runtime configuration without complete OpenSpec artifacts
- **THEN** the specification gate fails the change

### Requirement: Human approval precedes implementation
A human MUST approve strictly valid proposal, spec, design, and task artifacts before implementation work starts, and material artifact changes MUST receive renewed approval.

#### Scenario: Unapproved implementation
- **WHEN** implementation begins before approval is recorded
- **THEN** the change is non-compliant and MUST NOT merge

### Requirement: Strict artifact validation
All active and current-state specifications MUST pass strict OpenSpec validation, and every requirement MUST use normative language and include a testable scenario.

#### Scenario: Requirement has no scenario
- **WHEN** strict validation encounters a requirement without a `#### Scenario:`
- **THEN** validation fails and implementation or merge remains blocked

### Requirement: Current-state updates occur through archive
Future requirement changes MUST originate as deltas and MUST update `openspec/specs` through archive only after all tasks and verification are complete.

#### Scenario: Direct current-state edit
- **WHEN** a diff modifies current-state requirement content without archiving a change in the same diff
- **THEN** the specification gate fails

#### Scenario: Archived unchecked task
- **WHEN** an archived change contains an unchecked task
- **THEN** repository enforcement fails even if the OpenSpec CLI would only warn

### Requirement: CI specification gate
Continuous integration MUST strictly validate specifications and compare code changes with active or archived OpenSpec changes before running the existing Bun and Go jobs.

#### Scenario: Code and matching valid change
- **WHEN** a code diff includes a complete, valid OpenSpec change
- **THEN** the specification gate passes and downstream CI jobs may run

#### Scenario: Documentation-only change
- **WHEN** a diff changes only documentation, governance, or OpenSpec proposal artifacts
- **THEN** the gate does not require an additional OpenSpec change

### Requirement: Comparison base fails closed
The gate MUST use a reachable pull-request, push, explicit, or local merge base and MUST fail with remediation if no comparison base can be resolved.

#### Scenario: Shallow history prevents comparison
- **WHEN** no configured or fallback base commit is reachable
- **THEN** the gate fails and instructs the contributor to provide `SPEC_GATE_BASE` or full history

### Requirement: Emergency exemption is narrow and auditable
Only urgent security fixes and reverts MAY use `SPEC-EXEMPT: <substantive reason>`, and each use MUST be prominently reported and followed by a normal specification change.

#### Scenario: Valid emergency exemption
- **WHEN** the HEAD commit contains an exemption reason of at least 15 characters
- **THEN** the artifact-presence check is bypassed, existing specs remain validated, and CI prints the reason prominently

#### Scenario: Missing or trivial reason
- **WHEN** code changes include no complete OpenSpec change and the exemption reason is absent or too short
- **THEN** the gate fails

### Requirement: Historical documentation is non-normative
Existing designs, plans, guides, and fix reports MUST be treated as supporting context and MUST NOT override current-state capability specifications.

#### Scenario: Historical document conflicts with a spec
- **WHEN** historical prose disagrees with current behavior
- **THEN** contributors verify implementation and update the normative spec through a new OpenSpec change

