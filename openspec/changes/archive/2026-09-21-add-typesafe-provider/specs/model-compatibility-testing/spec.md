## ADDED Requirements

### Requirement: TypeSafe evaluation compatibility evidence
The maintained compatibility manifest SHALL include TypeSafe with native System
One execution and explicit not-applicable reasons for generation, streaming,
embeddings, and native tool loops. Deterministic tests SHALL exercise Noul, Choice,
and Score through the real gateway/provider boundaries and verify rejection of
incompatible generation shapes. Adding this capability MUST preserve the existing
five-turn coding-agent client-shape regression coverage.

#### Scenario: Mixed native evaluation fixture
- **WHEN** a deterministic request contains all three primitive types with structured state, instructions, and criteria
- **THEN** the fake upstream observes the original question identifiers and values and the client receives the full answers and usage

#### Scenario: Unsupported coding-agent operation
- **WHEN** compatibility tests exercise a TypeSafe model with Chat Completions, Messages, or Responses tool requests
- **THEN** those requests fail explicitly before upstream execution and are not counted as successful native tool-loop coverage

#### Scenario: Credentialed live smoke
- **WHEN** an explicitly enabled isolated live smoke has a TypeSafe API key
- **THEN** it verifies model discovery and one mixed-primitive evaluation through a fresh-port gateway and reports only sanitized outcomes

#### Scenario: Existing coding-agent coverage
- **WHEN** shared routing or ingestion changes for TypeSafe are verified
- **THEN** deterministic five-turn Claude Code, Codex CLI, Droid, and GitHub Copilot CLI client-shape checks pass, and corresponding bounded live checks run when prerequisites are available or record concrete skip reasons
