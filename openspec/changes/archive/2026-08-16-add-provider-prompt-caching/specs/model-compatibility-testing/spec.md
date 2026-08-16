## ADDED Requirements

### Requirement: Deterministic prompt-cache compatibility coverage
The deterministic compatibility matrix SHALL verify prompt-cache request preservation, provider-specific forwarding or omission, usage normalization, and streaming parity at each affected client, canonical, provider, and route boundary.

#### Scenario: Cache-bearing five-turn history
- **WHEN** the client-shape matrix executes a history containing at least five sequential tool calls and results with a stable marked prefix
- **THEN** every cache directive remains associated with its intended canonical element, every tool relationship remains ordered, and terminal continuation occurs without a sixth call

#### Scenario: Provider payload contract
- **WHEN** a cache-bearing canonical request executes against a fake upstream for each provider strategy
- **THEN** the exact payload demonstrates the strategy's declared forwarding or omission behavior without relying on a live service

### Requirement: Credential-gated live prompt-cache proof
The live model matrix SHALL offer an explicit prompt-cache scenario only for rows that declare a verified cache-usage contract and SHALL use two sequential requests to the same model, provider instance, and upstream shape with an identical sufficiently large prefix and a varying suffix.

#### Scenario: Declared live cache support
- **WHEN** a row declares prompt caching supported and the second request completes
- **THEN** normalized cache-read input is greater than zero, the exact-response cache did not satisfy the request, and zero or missing cache-read evidence is a failure rather than a pass

#### Scenario: No verified usage contract
- **WHEN** a provider or model cannot expose cache-read usage or has no verified request contract
- **THEN** the live combination is skipped or not applicable with a concrete reason and is not counted as cache-success evidence

### Requirement: Fresh-port prompt-cache verification
Baseline and post-change live verification SHALL each run in separate isolated gateway processes using automatically allocated loopback ports and SHALL report the actual port and sanitized outcomes.

#### Scenario: Post-change live run
- **WHEN** smoke and extended verification run after implementation
- **THEN** neither process reuses a fixed or previously running gateway port or imports normal OmniLLM runtime state

## MODIFIED Requirements

### Requirement: Coding-agent client tool-loop verification
Changes affecting ingestion, canonical tool calls or results, provider transforms, response streaming, or prompt-cache controls SHALL retain deterministic multi-turn tool-loop coverage for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider client shapes and SHALL run bounded live client smoke tests when the corresponding local client and model configuration are available.

#### Scenario: Deterministic client-shape coverage
- **WHEN** a prompt-cache or tool-call compatibility change is verified
- **THEN** automated tests exercise the affected Anthropic, Chat Completions, and Responses request shapes with at least five sequential calls, five correctly associated results, preserved cache semantics, and a terminal continuation without a sixth call

#### Scenario: Configured local clients
- **WHEN** Claude Code, Codex CLI, Droid, or GitHub Copilot CLI custom provider and a local OmniLLM model route are available
- **THEN** verification runs a bounded live smoke on an isolated fresh-port gateway that performs at least five sequential native tool calls, observes each result before issuing the next call, reaches a terminal assistant response, and exits successfully

#### Scenario: Live client unavailable
- **WHEN** a live smoke cannot run because its executable, authentication, safe temporary custom-provider configuration, model route, or gateway is unavailable
- **THEN** verification records the concrete skip reason while deterministic client-shape regression coverage remains required

#### Scenario: Live client produces incomplete loop
- **WHEN** a configured live client produces fewer than five calls, duplicate or missing markers, an association error, an unexpected sixth call, no terminal response, or a non-zero exit
- **THEN** verification reports failure rather than skip
