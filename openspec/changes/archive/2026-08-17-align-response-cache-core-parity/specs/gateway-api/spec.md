## ADDED Requirements

### Requirement: Responses exact-cache behavior
The `/v1/responses` endpoint SHALL apply the operator-enabled exact-response cache to non-streaming and streaming requests, SHALL preserve existing per-request bypass and disable controls, and SHALL execute the provider normally on a miss or incompatible cached value.

#### Scenario: Non-streaming Responses miss and hit
- **WHEN** two semantically identical non-streaming Responses requests execute while caching is enabled
- **THEN** the first successful provider result is marked as a miss and stored, and the second is marked as a hit and returned without another provider execution

#### Scenario: Streaming Responses miss and hit
- **WHEN** a complete Responses stream populates the cache and an equivalent streaming request follows
- **THEN** the second request emits valid Responses SSE events from the canonical entry with exactly one `response.completed` event

#### Scenario: Responses cache bypass
- **WHEN** a Responses request uses the existing bypass or refresh control
- **THEN** the cache read is skipped, ordinary provider execution occurs, and the successful complete result may refresh the entry

#### Scenario: Responses cache disabled per request
- **WHEN** a Responses request uses the existing off or disable control
- **THEN** neither a cache read nor a cache write occurs

#### Scenario: Incompatible cached value
- **WHEN** a cached canonical value cannot be converted into a valid Responses object or stream
- **THEN** the gateway does not commit a hit response and continues through ordinary provider execution

### Requirement: Cached Responses native-tool fidelity
Cached Responses replay SHALL preserve ordinary function calls and native custom tool calls in their respective public event and object vocabularies, including identifiers, declared names, raw input, optional namespace, ordering, and call-result relationships.

#### Scenario: Cached custom tool response
- **WHEN** a cached canonical response contains a native custom tool call
- **THEN** non-streaming Responses output contains `custom_tool_call` with the original call identifier, name, raw input, and namespace when present

#### Scenario: Cached custom tool stream
- **WHEN** a streaming Responses request hits a cached native custom tool call
- **THEN** the stream emits custom input delta and done events in order and does not emit function-argument events for that custom call

#### Scenario: Cached function tool remains unchanged
- **WHEN** a cached canonical response contains an ordinary function call
- **THEN** Responses replay continues emitting `function_call` items and function-argument events
