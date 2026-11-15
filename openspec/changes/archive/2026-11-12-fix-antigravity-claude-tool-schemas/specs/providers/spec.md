## ADDED Requirements

### Requirement: Antigravity Claude JSON Schema fidelity
Antigravity Claude generation SHALL transmit canonical tool parameter schemas through its full JSON Schema representation, preserving nested composition, discriminator constants, references, required properties, and constraints without mutating the canonical request. It MUST retain tool names, descriptions, ordering, and call/result history and MUST NOT remove tools to recover a schema rejection. Antigravity models outside the Claude family SHALL retain their existing tool representation.

#### Scenario: Nested SendMessage union
- **WHEN** a Claude request declares a tool whose parameter accepts a string or nested object alternatives with discriminator constants
- **THEN** the complete schema reaches the upstream JSON Schema field without conversion to the restricted OpenAPI schema field or loss of the alternatives and constants

#### Scenario: Provider-pinned Claude models and ingestion dialects
- **WHEN** Messages, Chat Completions, or Responses input routes to Antigravity Opus or Sonnet
- **THEN** the canonical tool schema uses the same full-schema transport for streaming and collected generation

#### Scenario: Missing schema and non-Claude models
- **WHEN** a tool has no parameter schema or an Antigravity request selects a Gemini model
- **THEN** existing missing-schema omission and Gemini tool representation behavior remain unchanged

#### Scenario: Multi-turn coding client
- **WHEN** a coding client supplies tool definitions and completes five sequential tool calls with observed results through Antigravity Claude
- **THEN** each call and result remains associated in order and the client receives a terminal continuation without an unintended extra tool call

### Requirement: Antigravity tool-call identity and continuation
Antigravity SHALL preserve upstream tool-call identifiers when supplied and SHALL generate non-empty distinct identifiers only when upstream omits them. Replayed calls and results MUST retain their canonical identifiers and association. A result without a tool name SHALL use the name of its matching earlier call when available. Responses that emit tool calls SHALL terminate with tool-use semantics, including when upstream reports ordinary completion or ends after a complete tool call.

#### Scenario: Native upstream identifier
- **WHEN** an Antigravity tool-call event includes an identifier
- **THEN** canonical output retains that identifier and the client continuation replays the corresponding call and result identifiers

#### Scenario: Missing upstream identifier
- **WHEN** upstream emits complete tool calls without identifiers
- **THEN** each call receives a distinct non-empty identifier that remains usable for its result continuation

#### Scenario: Unnamed tool result
- **WHEN** a result has a call identifier but omits its tool name
- **THEN** Antigravity resolves the name from the matching prior call rather than sending an empty result name or associating it with another call

#### Scenario: Upstream stop after a tool call
- **WHEN** a response emits a tool call followed by STOP or a clean stream end
- **THEN** its canonical stop reason is tool use and a subsequent result can continue the conversation

#### Scenario: Ordinary text completion
- **WHEN** a response emits only text and completes normally
- **THEN** its ordinary end-turn stop reason remains unchanged
