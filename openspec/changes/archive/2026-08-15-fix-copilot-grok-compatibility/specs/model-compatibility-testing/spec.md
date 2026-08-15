## ADDED Requirements

### Requirement: Copilot Grok compatibility evidence
The compatibility suite SHALL provide isolated deterministic and credential-gated live evidence for Grok models advertised by GitHub Copilot without embedding credentials or importing the operator's normal OmniLLM runtime state.

#### Scenario: Discover applicable Grok models
- **WHEN** a credentialed Copilot live-matrix run begins
- **THEN** the tested Grok identifiers, capabilities, and supported endpoints are derived from the authenticated Copilot provider catalog rather than a direct xAI endpoint assumption

#### Scenario: Fresh-port Grok live run
- **WHEN** Copilot Grok live compatibility is exercised
- **THEN** OmniLLM runs with a temporary home and database on a newly allocated loopback port and reports sanitized model, shape, scenario, duration, and outcome evidence

#### Scenario: Grok smoke coverage
- **WHEN** smoke mode runs for an applicable Copilot Grok model
- **THEN** model availability, plain generation, streaming completion, and one tool-call/result replay execute for each declared gateway shape

#### Scenario: Grok extended coverage
- **WHEN** extended mode runs for an applicable Copilot Grok model
- **THEN** five ordered tool cycles with a terminal continuation, parallel tools, a large result, a long-lived stream, and caller cancellation execute or are reported not applicable with a concrete catalog- or behavior-based reason

#### Scenario: Deterministic reproduced defect
- **WHEN** the live matrix exposes a Copilot Grok compatibility failure
- **THEN** a fake-upstream regression reproduces the same endpoint, payload, stream, or routing condition for every affected API shape and verifies no regression to existing Copilot strategy rows
