## ADDED Requirements

### Requirement: Claude Code Plan agent-type compatibility
For an Anthropic Messages request whose `User-Agent` contains a parsed `claude-cli/<version>` product token, the gateway SHALL repair a completed `Agent` tool call whose top-level string `subagent_type` value exactly equals `Plan-scriptPath not found: plan file is managed by plan mode system, use Plan agent output only.` to the string `Plan` only when the request's declared `Agent` input schema directly allows `Plan` for `subagent_type` through a string `enum` or `const`. The gateway SHALL preserve the value unchanged for every other client, route, tool name, property, value, or schema shape. Live non-streaming, live streaming, and cache-replayed responses SHALL expose contract-equivalent tool calls.

#### Scenario: Claude Code exact Plan-mode sentinel
- **WHEN** a Claude Code Messages request declares an `Agent.subagent_type` schema that directly allows `Plan` and the completed call contains the exact known Plan-mode sentinel
- **THEN** the client receives the same tool call with `subagent_type` equal to `Plan` and every other argument unchanged

#### Scenario: Streaming Plan agent call
- **WHEN** the exact sentinel arrives across one or more streamed argument deltas for a qualifying Claude Code request
- **THEN** the gateway repairs the completed indexed call before emission without changing its identifier, order, or other arguments

#### Scenario: Non-Claude Code caller
- **WHEN** an otherwise identical Messages request does not contain a parsed `claude-cli/<version>` product token
- **THEN** the gateway preserves the model-emitted `subagent_type` unchanged

#### Scenario: Schema does not authorize Plan
- **WHEN** the `Agent` tool is absent or its directly declared `subagent_type` property does not explicitly allow `Plan` through a string `enum` or `const`
- **THEN** the gateway preserves the model-emitted value unchanged

#### Scenario: Near-match diagnostic
- **WHEN** `subagent_type` differs from the exact known sentinel by any prefix, suffix, character, case, type, or property location
- **THEN** the gateway preserves the model-emitted value unchanged

#### Scenario: Cached response parity
- **WHEN** a qualifying Claude Code request receives an affected tool call from response cache
- **THEN** the cached response exposes the same repaired call as a live response, or the gateway bypasses a cache entry that cannot safely satisfy the repair contract

### Requirement: Metadata-only Claude Code compatibility diagnostics
When the Claude Code Plan agent-type repair is applied, the gateway SHALL emit one structured diagnostic per repaired call containing applicable request, API-shape, model, provider, tool, call, stream, cache, and stable repair-reason metadata without emitting tool arguments, prompt content, or tool-result content.

#### Scenario: Live repair diagnostic
- **WHEN** the gateway repairs an affected live non-streaming or streaming call
- **THEN** one diagnostic identifies the repair and call using metadata only

#### Scenario: Cache repair diagnostic
- **WHEN** the gateway repairs or bypasses an affected cached call to preserve contract parity
- **THEN** the diagnostic identifies the cache decision without including cached arguments or content
