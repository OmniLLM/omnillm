## Why

Codex CLI 0.147 declares its callable freeform tool as `exec` inside an `additional_tools` namespace. OmniLLM currently changes the name to `functions__exec` and coerces the custom definition/call into function-tool JSON. Fixing the name alone lets Codex recognize the tool, but it then rejects the returned function payload as incompatible. Native custom-tool kind and raw input must survive the Responses path.

## What Changes

- Preserve nested `additional_tools` declared names and deterministic duplicate-name handling.
- Extend CIF additively to retain function-versus-custom kind, custom format, raw input, namespace, and original custom output while keeping existing function-compatible fallback fields.
- Preserve native custom definitions, history, non-streaming calls, and streaming input deltas through the Copilot Responses provider.
- Emit native `custom_tool_call` items and custom input SSE events from the public Responses API.
- Keep Chat Completions, Anthropic, and non-Copilot provider behavior backward compatible through existing fallback arguments/results.
- Add deterministic ingestion, CIF, Copilot, serialization, and route tests plus live Claude Code, Codex CLI, and Droid multi-turn verification.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cif-translation`: Preserve custom-tool identity and raw values while retaining function-compatible fallbacks.
- `gateway-api`: Emit native Responses custom-tool items and streaming events.
- `providers`: Preserve native custom tools through Copilot Responses requests and responses.
- `model-compatibility-testing`: Verify Codex custom `exec` round trips and all three coding clients complete multi-turn tool loops.

## Impact

- CIF tool definitions, calls, results, stream deltas, and JSON serialization.
- Responses ingestion and additional-tool extraction.
- Copilot Responses request building, non-streaming parsing, and SSE parsing.
- Public Responses non-streaming and streaming serialization.
- Generic stream collection/normalization helpers only where required to retain new additive CIF fields.
- No dependency, database, or configuration migration.
