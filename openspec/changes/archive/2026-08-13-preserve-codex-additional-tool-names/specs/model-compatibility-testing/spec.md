## ADDED Requirements

### Requirement: Codex Responses custom-tool compatibility
The compatibility suite SHALL verify that Codex custom `exec` definitions and calls retain native kind, name, and raw input through Responses ingestion, Copilot execution, and Responses serialization.

#### Scenario: Deterministic exec round trip
- **WHEN** a Codex-shaped Responses fixture declares namespace `functions` custom tool `exec` and the provider calls it
- **THEN** every deterministic boundary exposes native custom kind, name `exec`, exact raw input, and the correct call identifier

#### Scenario: Live Codex multi-turn tool loop
- **WHEN** Codex CLI and `jzhu/gpt-5.6-sol` are locally available through OmniLLM
- **THEN** a bounded smoke test executes at least two sequential `exec` calls and reaches a terminal response without unsupported-name or incompatible-payload errors

#### Scenario: Other coding clients remain compatible
- **WHEN** native Codex custom-tool fidelity is verified
- **THEN** bounded Claude Code and Droid multi-turn tool-loop tests continue reaching terminal responses
