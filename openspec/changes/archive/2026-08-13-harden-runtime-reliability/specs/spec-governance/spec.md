## ADDED Requirements

### Requirement: Actionable dead-code analysis
The repository SHALL define the actual frontend, desktop, script, and test entrypoints for dead-code analysis so the configured command distinguishes reachable project code and declared dependencies from genuine unused code without structural entrypoint or path-alias false positives.

#### Scenario: Dead-code analysis on the maintained tree
- **WHEN** the dead-code analysis command runs against a clean maintained repository
- **THEN** it resolves configured entrypoints and path aliases and exits successfully when no genuine unused items remain

#### Scenario: Unused reachable-area export is introduced
- **WHEN** a new export or dependency is not reachable from any configured production, script, or test entrypoint
- **THEN** dead-code analysis reports it for remediation
