## ADDED Requirements

### Requirement: Canonical-toolchain security analysis

Repository security analysis MUST build Go code with the canonical toolchain
declared by `go.mod` and MUST retain analysis for Actions,
JavaScript/TypeScript, and Rust.

#### Scenario: Go version exceeds hosted default

- **WHEN** GitHub's default CodeQL Go runtime is older than the module requirement
- **THEN** the repository workflow installs the `go.mod` toolchain and manually builds Go code for CodeQL extraction

#### Scenario: Removed binary migration note

- **WHEN** current documentation explains how uninstall removes a stale legacy executable
- **THEN** cleanup regression coverage permits the migration note while still rejecting active entrypoint or invocation documentation
