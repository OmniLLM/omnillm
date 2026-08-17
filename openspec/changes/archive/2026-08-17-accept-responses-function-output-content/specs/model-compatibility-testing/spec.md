## MODIFIED Requirements

### Requirement: Coding-agent client tool-loop verification
Changes affecting ingestion, canonical tool calls or results, provider transforms, response streaming, or prompt-cache controls SHALL retain deterministic multi-turn tool-loop coverage for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider client shapes and SHALL run bounded live client smoke tests when the corresponding local client and model configuration are available. When an affected client shape supports structured function-result content, verification SHALL include a standards-compliant non-string output continuation in addition to the ordinary string-result loop.

#### Scenario: Deterministic client-shape coverage
- **WHEN** a prompt-cache or tool-call compatibility change is verified
- **THEN** automated tests exercise the affected Anthropic, Chat Completions, and Responses request shapes with at least five sequential calls, five correctly associated results, preserved cache semantics, and a terminal continuation without a sixth call

#### Scenario: Deterministic structured function output
- **WHEN** Responses function-result ingestion or replay is verified
- **THEN** automated tests exercise string, empty, text-list, mixed multimodal-list, and malformed-list outputs through ingestion, canonical preservation, native Responses replay, textual provider fallback, and route dispatch

#### Scenario: Configured local clients
- **WHEN** Claude Code, Codex CLI, Droid, or GitHub Copilot CLI custom provider and a local OmniLLM model route are available
- **THEN** verification runs a bounded live smoke on an isolated fresh-port gateway that performs at least five sequential native tool calls, observes each result before issuing the next call, reaches a terminal assistant response, and exits successfully

#### Scenario: Configured structured-output client
- **WHEN** a locally available coding client can deterministically emit a standards-compliant structured `function_call_output`
- **THEN** verification additionally observes successful continuation after that non-string result and a terminal response

#### Scenario: Live client unavailable
- **WHEN** a live smoke cannot run because its executable, authentication, safe temporary custom-provider configuration, model route, gateway, or deterministic structured-output mechanism is unavailable
- **THEN** verification records the concrete skip reason while deterministic client-shape regression coverage remains required

#### Scenario: Live client produces incomplete loop
- **WHEN** a configured live client produces fewer than five calls, duplicate or missing markers, an association error, an unexpected sixth call, no terminal response, or a non-zero exit
- **THEN** verification reports failure rather than skip
