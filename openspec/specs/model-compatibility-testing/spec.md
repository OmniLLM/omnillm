# model-compatibility-testing Specification

## Purpose
Defines the evidence required to verify that supported client API shapes, provider execution strategies, and compatible models preserve generation, streaming, and agentic tool-use contracts.
## Requirements
### Requirement: Layered deterministic compatibility matrix
The test suite SHALL verify model compatibility through separate client-shape-to-canonical, canonical-to-provider, and thin end-to-end matrix layers so that every supported API shape and provider execution strategy is covered without an unbounded Cartesian suite.

#### Scenario: Supported inbound generation shapes
- **WHEN** deterministic compatibility tests run
- **THEN** OpenAI Chat Completions, Anthropic Messages, and OpenAI Responses shapes each execute plain, streaming, and tool-use semantic fixtures

#### Scenario: Supported provider strategies
- **WHEN** deterministic provider contract tests run
- **THEN** every supported provider type and each distinct upstream execution strategy has at least one representative compatible model row

#### Scenario: Dynamic upstream shape
- **WHEN** a provider can select Chat Completions or Responses according to model or configuration
- **THEN** the matrix includes representative rows for every supported upstream shape

### Requirement: Agentic tool-loop compatibility
The matrix SHALL verify that applicable models preserve tool definitions, call identifiers, names, arguments, results, ordering, error markers, and assistant-turn structure through repeated and parallel tool-use turns.

#### Scenario: Sequential tool loop
- **WHEN** an applicable model executes at least five sequential tool cycles
- **THEN** five distinct calls and results remain associated in order and the final response terminates without an unexpected sixth tool call

#### Scenario: Parallel streamed tools
- **WHEN** an applicable streaming model emits interleaved arguments for multiple tool calls
- **THEN** arguments are accumulated by provider index or call identifier without mixing calls, and each result is replayed to the correct call

#### Scenario: Long tool execution and result
- **WHEN** a tool call remains active beyond an ordinary short test interval or returns a large deterministic result
- **THEN** the request remains valid within its configured budget and the tool result is preserved without truncation or relationship loss

#### Scenario: Tool error result
- **WHEN** a tool result is marked as an execution error
- **THEN** its error relationship is preserved across the client shape and provider contract without leaking its content into diagnostic output

### Requirement: Streaming and timeout invariants
Compatibility tests SHALL verify dialect-specific stream event vocabularies, terminal events, configured long-request budgets, cancellation propagation, and absence of duplicate output or unintended retries.

#### Scenario: Normal stream completion
- **WHEN** a compatible model completes a stream
- **THEN** the client receives the correct terminal event for its requested API shape and no duplicate tool-call or text events

#### Scenario: Long-lived stream
- **WHEN** response headers arrive within budget and the stream remains active while producing valid events
- **THEN** the stream is not terminated by an ordinary non-stream request timeout

#### Scenario: Caller cancellation
- **WHEN** the caller cancels an in-flight model request
- **THEN** provider execution is cancelled and no subsequent failover candidate or retry emits output

### Requirement: Credential-gated live model matrix
The repository SHALL provide an explicit opt-in live matrix that exercises configured API-shape and compatible-model rows against real upstream providers while isolating runtime state and credentials from the normal OmniLLM installation.

#### Scenario: Live testing not enabled
- **WHEN** the live matrix command runs without explicit opt-in
- **THEN** it exits successfully before building, launching the gateway, reading credentials, or contacting an upstream service and reports that it was skipped

#### Scenario: Required credential unavailable
- **WHEN** a configured live row lacks a required credential or token bundle
- **THEN** that row is reported as skipped with a reason and other runnable rows continue

#### Scenario: Supplied credential fails
- **WHEN** a live row has all declared credentials but authentication or model execution fails
- **THEN** the row is reported as failed rather than skipped

#### Scenario: Unsupported combination
- **WHEN** a model row explicitly declares that a shape or capability is unsupported
- **THEN** the combination is reported as not applicable and is not counted as evidence of failure or success

#### Scenario: Isolated live execution
- **WHEN** the live matrix launches OmniLLM
- **THEN** it uses temporary runtime state, an automatically allocated loopback port, bounded requests, and credential references that do not expose secret values in console or machine-readable output

### Requirement: Compatibility result reporting
Matrix execution SHALL produce a human-readable summary and a machine-readable report that distinguishes pass, fail, skip, and not-applicable outcomes for every planned row.

#### Scenario: Matrix finishes
- **WHEN** deterministic or live matrix execution completes
- **THEN** the report identifies each API shape, provider, model, scenario, execution mode, outcome, duration, and sanitized failure reason

#### Scenario: Incomplete planned coverage
- **WHEN** a deterministic row expected by the maintained capability manifest does not execute
- **THEN** the deterministic matrix fails rather than silently reducing coverage

### Requirement: Droid custom-tool compatibility fixture
The deterministic compatibility suite SHALL include a sanitized Droid-style Responses history containing a raw `ApplyPatch` custom call and its associated result, and SHALL verify canonical normalization and route dispatch.

#### Scenario: Sanitized ApplyPatch replay
- **WHEN** the compatibility suite submits the Droid-style custom-tool fixture to `/v1/responses`
- **THEN** the request succeeds, provider dispatch occurs, the raw patch input is preserved under the canonical `input` argument, and the result remains associated with the originating `call_id`

#### Scenario: Existing function-tool fixtures
- **WHEN** the compatibility suite runs existing Responses function-call histories after custom-tool support is added
- **THEN** their canonical messages, arguments, ordering, and call-result relationships remain unchanged

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
