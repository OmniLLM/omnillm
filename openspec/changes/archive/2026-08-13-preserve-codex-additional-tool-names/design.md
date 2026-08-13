## Context

See `proposal.md` for motivation. Codex registers `exec` as a Responses custom/freeform tool with raw string input. OmniLLM currently loses custom kind and format in CIF, sends a function definition to Copilot, parses only function-call output, and emits function argument events. Codex therefore first rejected the invented name and, after name preservation, rejected the function-shaped payload.

## Goals / Non-Goals

**Goals:**

- Preserve native custom-tool identity through the Codex → OmniLLM → Copilot Responses → OmniLLM → Codex path.
- Keep all new CIF fields additive with function as the zero-value default.
- Retain existing synthetic schema, arguments, and result text as fallback data for Claude, Droid, and non-native provider paths.
- Support non-streaming and streaming custom calls with exact raw input and stable call association.

**Non-Goals:**

- Native custom-tool support for every Responses-like provider in this change.
- Native custom tools in Chat Completions or Anthropic wire protocols.
- Removal of focused compatibility normalization added for Droid.

## Decisions

### Add a backward-compatible CIF tool kind

Introduce a tool-kind discriminator whose zero value means function and whose explicit custom value enables native Responses behavior. Add custom format to tool definitions, raw input and namespace to tool calls, original custom output to results, and a raw custom-input stream delta. Existing JSON with none of these fields continues behaving identically.

Using only extension maps was rejected because core serializers and stream processors need typed invariants. Replacing existing function fields was rejected because it would break every adapter and cached shape.

### Retain dual native and fallback representations

Custom calls preserve exact raw input while also storing `{"input": raw}` arguments. Custom results preserve their original string/list value while also storing current normalized text. Custom definitions preserve format while retaining the synthetic input schema.

This lets Copilot Responses use native fields while existing Claude/Droid/chat paths continue consuming established fields.

### Preserve declared names and deduplicate first-wins

Nested `additional_tools` namespace labels are transport grouping and are not prepended. The first definition for a declared name is retained; later duplicates are ignored deterministically because CIF and provider tool lookup are globally name-based.

### Implement native behavior only in Copilot Responses and public Responses

Copilot is the provider selected by `jzhu/gpt-5.6-sol` and the live failure path. Its builder, non-streaming parser, stop-reason logic, and SSE parser branch on tool kind. Public Responses serialization emits native custom items and custom input delta/done events. Other provider paths use fallbacks unchanged.

### Keep custom input out of JSON argument normalization

Raw custom-input deltas use a distinct CIF delta type. Generic argument normalization and stream collection must never parse custom input as JSON; collectors append raw text by content index and preserve kind/namespace.

## Risks / Trade-offs

- **Dual representations can diverge** → Construct both together at ingestion/provider parsing and cover consistency in tests.
- **Generic stream helpers may drop additive fields** → Audit canonical marshaling, collection, caching, and argument normalization with focused tests.
- **Non-Copilot failover is intentionally lossy** → Preserve existing fallback behavior and scope native guarantees to the Copilot Responses path.
- **Provider custom SSE variants may differ** → Cover output-item added/done plus custom-input delta/done and avoid duplicate done content.

## Migration Plan

1. Add CIF fields with omitempty and function zero-value defaults.
2. Add deterministic tests before switching Copilot/public Responses branches.
3. Run live Codex two-call acceptance, then Claude Code and Droid regressions.
4. Roll back the change if needed; old serialized CIF remains readable because all new fields are optional.
