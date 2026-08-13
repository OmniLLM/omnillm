## MODIFIED Requirements

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

### Requirement: Coding-agent client tool-loop verification
Changes affecting ingestion, canonical tool calls or results, provider transforms, or streaming SHALL retain deterministic multi-turn tool-loop coverage for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider client shapes and SHALL run bounded live client smoke tests when the corresponding local client and model configuration are available.

#### Scenario: Deterministic client-shape coverage
- **WHEN** a tool-call compatibility change is verified
- **THEN** automated tests exercise the affected Anthropic, Chat Completions, and Responses request shapes with at least five sequential calls, five correctly associated results, and a terminal continuation without a sixth call

#### Scenario: Configured local clients
- **WHEN** Claude Code, Codex CLI, Droid, or GitHub Copilot CLI custom provider and a local OmniLLM model route are available
- **THEN** verification runs a bounded live smoke that performs at least five sequential native tool calls, observes each result before issuing the next call, reaches a terminal assistant response, and exits successfully

#### Scenario: Live client unavailable
- **WHEN** a live smoke cannot run because its executable, authentication, custom-provider capability, model route, or gateway is unavailable
- **THEN** verification records the concrete skip reason while deterministic client-shape regression coverage remains required

#### Scenario: Live client produces incomplete loop
- **WHEN** a configured live client produces fewer than five calls, duplicate or missing markers, an association error, an unexpected sixth call, no terminal response, or a non-zero exit
- **THEN** verification reports failure rather than skip
