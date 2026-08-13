## ADDED Requirements

### Requirement: Droid custom-tool compatibility fixture
The deterministic compatibility suite SHALL include a sanitized Droid-style Responses history containing a raw `ApplyPatch` custom call and its associated result, and SHALL verify canonical normalization and route dispatch.

#### Scenario: Sanitized ApplyPatch replay
- **WHEN** the compatibility suite submits the Droid-style custom-tool fixture to `/v1/responses`
- **THEN** the request succeeds, provider dispatch occurs, the raw patch input is preserved under the canonical `input` argument, and the result remains associated with the originating `call_id`

#### Scenario: Existing function-tool fixtures
- **WHEN** the compatibility suite runs existing Responses function-call histories after custom-tool support is added
- **THEN** their canonical messages, arguments, ordering, and call-result relationships remain unchanged

### Requirement: Coding-agent client tool-loop verification
Changes affecting ingestion, canonical tool calls or results, provider transforms, or streaming SHALL retain deterministic multi-turn tool-loop coverage for Claude Code, Codex CLI, and Droid client shapes and SHALL run bounded live client smoke tests when the corresponding local client and model configuration are available.

#### Scenario: Deterministic client-shape coverage
- **WHEN** a tool-call compatibility change is verified
- **THEN** automated tests exercise the affected Anthropic, Chat Completions, and Responses request shapes, including at least two sequential calls and a terminal continuation

#### Scenario: Configured local clients
- **WHEN** Claude Code, Codex CLI, or Droid and a local OmniLLM model route are available
- **THEN** verification runs a bounded live smoke prompt that performs at least two sequential tool calls and reaches a terminal response

#### Scenario: Live client unavailable
- **WHEN** a live client smoke cannot run because the client, authentication, model route, or gateway is unavailable
- **THEN** verification records the concrete skip reason while deterministic client-shape regression coverage remains required
