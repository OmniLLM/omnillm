## Why

Claude Code stopped completing tool-bearing requests when its MCP tool set included `mcp__plugin_ui-color-palette_ui-color-palette__create_color_harmony`. That tool declares a draft-07 RGB tuple using array-valued `items`. OmniLLM removes the draft declaration during Anthropic ingestion but leaves the legacy tuple syntax intact. Copilot's Claude backend requires draft 2020-12 and rejects the entire request, surfaced to the client as HTTP 502.

A bounded live comparison on the same installed gateway and `claude-sonnet-5.5` route reproduced HTTP 502 with the original tool and HTTP 200 with a semantically equivalent `prefixItems` tuple. Neither request executed a tool. See `investigation.md` for evidence and limits.

## What Changes

- Normalize legacy tuple schemas during Anthropic tool ingestion into equivalent draft 2020-12 tuple syntax.
- Preserve positional validation and trailing-item policy: array-valued `items` becomes `prefixItems`, and its associated `additionalItems` becomes `items` when present.
- Traverse schema-bearing keywords without applying tuple rewrites to literal values, examples, or property-name dictionaries as though they were schemas.
- Preserve tool names, descriptions, ordering, cache controls, and call/result relationships; do not remove tools or weaken validation to bypass failures.
- Add deterministic regression and client compatibility coverage and run bounded live checks where prerequisites exist.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cif-translation`: Anthropic tool ingestion normalizes legacy tuple syntax without changing accepted argument values.

## Impact

Affected areas are Anthropic ingestion, schema normalization tests, and compatibility verification. No dependencies or persistent configuration changes are planned. The separate unavailable model spelling `claude-opus-5-5` is outside this change; the live catalog advertises `claude-opus-5.5`.

Implementation and deployment require approval of this proposal, delta spec, design, and tasks after strict validation.
