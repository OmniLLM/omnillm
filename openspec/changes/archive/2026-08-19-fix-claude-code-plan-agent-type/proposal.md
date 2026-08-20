## Why

Claude Code 2.1.235 can expose its Plan agent to a non-Claude model with the model-visible label `Plan-scriptPath not found: plan file is managed by plan mode system, use Plan agent output only.` The model can then copy that label into the `Agent` tool's `subagent_type` argument. Claude Code rejects the resulting otherwise valid tool call because the registered agent type is `Plan`, causing repeated local execution failures and preventing Plan-mode delegation through OmniLLM.

## What Changes

- Add a Claude Code compatibility repair on the Anthropic Messages response path for the known malformed Plan-mode `Agent.subagent_type` sentinel.
- Repair the malformed value to `Plan` only when the request declares the `Agent` tool and its input schema confirms `Plan` as an allowed `subagent_type` value.
- Apply the same behavior to non-streaming responses, streaming responses, and response-cache replay.
- Preserve every other tool name, argument, client, malformed value, and schema-uncertain value unchanged.
- Emit metadata-only diagnostics when the repair is applied without logging tool arguments or prompt content.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cif-translation`: Permit one schema-verified Claude Code compatibility repair while retaining the general tool-argument fidelity contract.
- `gateway-api`: Define the Claude Code Messages-route behavior and parity across live and cached responses.

## Impact

- Affects client-facing Anthropic Messages responses identified as Claude Code requests.
- Affects shared completed tool-argument normalization, Anthropic streaming, and response-cache replay.
- Enables Claude Code Plan-mode agent execution when non-Claude models reproduce the known internal `scriptPath` diagnostic in `Agent.subagent_type`.
- Does not change public request schemas, provider selection, dependencies, persisted data, or behavior for non-Claude Code clients.
