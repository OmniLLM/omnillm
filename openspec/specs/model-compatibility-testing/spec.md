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
- **WHEN** an applicable model executes at least three sequential tool cycles
- **THEN** every result remains associated with its originating call and the final response terminates without an unexpected additional tool call

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
