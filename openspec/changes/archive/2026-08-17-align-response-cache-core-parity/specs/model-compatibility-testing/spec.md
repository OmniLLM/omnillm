## ADDED Requirements

### Requirement: Deterministic exact-response cache compatibility coverage
The deterministic compatibility matrix SHALL verify exact-response cache population and replay for Chat Completions, Anthropic Messages, and Responses client shapes in non-streaming and streaming modes, including omitted and nonzero sampling controls, function tools, native custom tools, structured results, and complete multi-turn histories.

#### Scenario: Three-shape miss and hit
- **WHEN** each supported generation shape submits the same semantic request twice with the opt-in cache enabled
- **THEN** the first successful request is a miss, the second is a hit, the upstream executes exactly once, and both responses preserve the requested dialect

#### Scenario: Streaming replay uniqueness
- **WHEN** a cached canonical response is replayed as a stream in any supported dialect
- **THEN** text and tool output are not duplicated and the dialect emits exactly one terminal event

#### Scenario: Five-turn cache-bearing history
- **WHEN** Claude Code, Codex CLI, Droid, or GitHub Copilot CLI custom-provider fixtures exercise a history with five sequential native tool calls and five observed results through cache population and replay
- **THEN** call identifiers, names, function arguments or custom raw inputs, namespaces, result relationships, and ordering remain intact and terminal continuation occurs without a sixth call

#### Scenario: Responses state isolation
- **WHEN** otherwise identical Responses fixtures use distinct `previous_response_id` values
- **THEN** deterministic tests prove that one chain cannot receive the other chain's cached response

#### Scenario: Provider prompt-cache distinction
- **WHEN** exact-response cache compatibility tests inspect usage or metering
- **THEN** exact-response hits remain distinguishable from provider prompt-cache hits, misses, reads, and writes

### Requirement: Credential-gated live exact-response cache proof
The live compatibility workflow SHALL offer an explicit exact-response cache scenario for supported model and client-shape rows, SHALL run against isolated runtime state and Redis configuration, and SHALL remain separate from provider prompt-cache proof.

#### Scenario: Live exact-cache request pair
- **WHEN** an opted-in live row runs exact-response cache verification
- **THEN** two semantically identical requests with omitted sampling controls produce a miss followed by a hit, preserve semantic output, and report sanitized timing and outcome evidence

#### Scenario: Live exact-cache stream
- **WHEN** a supported live row verifies streaming replay
- **THEN** the hit stream uses the requested dialect, produces no duplicate output, and emits exactly one terminal event

#### Scenario: Configured coding clients
- **WHEN** Claude Code, Codex CLI, Droid, or GitHub Copilot CLI custom provider and a safe local model route are available
- **THEN** a bounded fresh-port smoke uses isolated cache state, performs at least five sequential native tool calls with observed results, reaches a terminal response, demonstrates exact cache behavior where applicable, and exits successfully

#### Scenario: Live prerequisite unavailable
- **WHEN** a client executable, authentication, safe temporary custom-provider configuration, model route, gateway, disposable Redis service, or deterministic scenario is unavailable
- **THEN** verification records that concrete skip reason while deterministic coverage remains mandatory

#### Scenario: Configured live loop is incomplete
- **WHEN** a configured client produces fewer than five calls, loses or duplicates a result, emits an unexpected sixth call, lacks a terminal response, fails exact-cache assertions, or exits nonzero
- **THEN** verification reports failure rather than skip
