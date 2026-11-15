## Why

Claude Code requests to Antigravity Claude fail before generation with an upstream draft-2020-12 tool-schema rejection, although plain prompts succeed. A schema-only capture reproduces the failure with the built-in SendMessage tool; its input schema and OmniLLM's locally transformed schema both validate as JSON Schema, indicating an incompatibility at the upstream schema representation boundary.

Native-client verification then exposed a second blocker: the first tool call succeeds, but replay drops its identifier and the continuation is rejected with `tool_use.id: Field required`.

## What Changes

- Preserve Claude tool schemas through Antigravity's JSON-schema-capable function declaration field instead of the restricted Gemini/OpenAPI schema representation, as verified by a controlled upstream comparison.
- Preserve nested unions, discriminator constants, references, constraints, tool names, and call/result history without dropping tools or weakening their schemas to obtain a response.
- Preserve upstream tool-call identifiers, carry call/result identifiers through Antigravity history, resolve omitted result names from matching calls, and report tool-use termination when a response contains tool calls.
- Prevent the gateway's non-streaming fallback from immediately replaying Antigravity HTTP 4xx failures that switching response mode cannot repair.
- Add deterministic provider and cross-client regression coverage and verify both Antigravity Claude models with real Claude Code five-call tool loops before delivery.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providers`: Antigravity Claude function-schema fidelity, supported full-schema transport, and tool identity/continuation correctness.
- `gateway-api`: Provider-declared rejection of inappropriate non-streaming fallback.

## Impact

Changes the Antigravity Claude outbound tool schema envelope, Antigravity call/result history and stream parsing, and shared gateway fallback eligibility. Gemini-family Antigravity requests retain their existing schema representation; their history also retains tool IDs. The separate Google provider converter remains unchanged. Existing HTTP error envelopes, model selection, credentials, database schema, and other providers' fallback defaults remain compatible. No new dependencies are planned. Temporary diagnostics retain tool definitions only; credentials, conversation content, and tool execution payloads must not be committed.
