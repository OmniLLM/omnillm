## Context

See `proposal.md` for motivation. Anthropic-compatible requests already carry each tool's JSON input schema into CIF, and provider adapters forward those schemas upstream. Provider responses then return tool arguments as complete JSON or streamed fragments, but current response translation treats arguments as schema-independent payloads.

The failure occurs when a model emits `""` for an optional object property. Some MCP servers distinguish an omitted property from a present invalid empty value. The solution crosses CIF metadata, provider response assembly, outbound serialization, and cached stream replay, while retaining the provider-index isolation introduced for interleaved tool calls.

## Goals / Non-Goals

**Goals:**

- Apply one provider-neutral normalization rule using the selected tool's declared JSON schema.
- Preserve required empty strings and all values that cannot be safely classified as optional empty strings.
- Produce equivalent normalized arguments for non-streaming, streaming, and cache-replayed responses.
- Retain tool-call identity, provider-index isolation, and emission order.

**Non-Goals:**

- Special-case Slack tool names or `context_channel_id`.
- Infer semantic defaults beyond the declared `required` property list.
- Recursively remove empty strings from nested objects without the corresponding nested schema.
- Rewrite tool-result content or caller-supplied historical tool calls.
- Repair malformed JSON beyond existing parser behavior.

## Decisions

### Normalize only completed argument objects

Accumulate each tool call's argument fragments by its existing provider index and normalize only after the JSON object is complete or the stream terminates. This avoids attempting to classify properties from incomplete JSON and ensures a property is removed atomically rather than by rewriting already-emitted fragments.

Alternative considered: filter individual string fragments during streaming. Rejected because JSON keys and values can span fragments, and previously emitted bytes cannot be safely retracted.

### Use the declared tool schema and preserve on uncertainty

Resolve the emitted tool name to the request's canonical tool declaration. Parse the top-level object schema's `required` list and remove only top-level properties whose value is exactly an empty string and whose property is declared but not required. If the tool, property, schema, or completed arguments cannot be resolved or parsed, preserve the original arguments.

This conservative fallback prevents accidental mutation of unknown extension schemas, malformed provider output, and required values.

Alternative considered: remove every empty-string property. Rejected because an empty string can be a valid required value and because undeclared properties may be intentionally forwarded.

### Keep normalization provider-neutral

Place the normalization behavior in shared translation logic that has access to canonical request tools, rather than matching Copilot or Slack identifiers. Provider adapters remain responsible for correctly assembling complete arguments and preserving call identity.

Alternative considered: normalize only in the Copilot adapter or hard-code Slack's `context_channel_id`. Rejected because any provider/model can emit empty placeholders and any MCP server can reject them.

### Preserve indexed stream identity and cache equivalence

Any buffering introduced for normalization must remain keyed by provider tool-call index and must not restore a single mutable current-block cursor. Response items retain their first-announcement order. A call that reaches an explicit block stop or index-reuse boundary flushes immediately before that boundary; when multiple calls remain open at stream termination, they flush in first-announcement order. Cache accumulation stores the same normalized completed arguments used for live emission so replay does not reintroduce omitted values or normalize a second time with different metadata.

## Risks / Trade-offs

- **[Risk] Buffering arguments until completion reduces incremental argument streaming.** → Limit buffering to tool-argument payloads. Flush at `CIFContentBlockStop` when available, immediately before index reuse, or at stream termination; Copilot chat tool arguments without block-stop events remain buffered until stream termination.
- **[Risk] A provider can emit a tool name that does not match a request declaration.** → Preserve arguments unchanged when schema resolution is uncertain.
- **[Risk] JSON Schema permits nested and composed schemas.** → Scope the initial rule to directly declared object properties and their explicit `required` list; preserve all other structures.
- **[Risk] Stream termination may arrive with incomplete JSON.** → Preserve existing malformed/incomplete argument handling rather than dropping data.
- **[Risk] Cache and live stream paths could diverge.** → Add paired regression tests asserting identical normalized arguments and ordering.

## Migration Plan

No persisted-data or configuration migration is required. Deploy as a backward-compatible response-translation correction. Rollback consists of reverting the normalization and associated buffering; existing cached responses remain readable because tool arguments stay JSON-compatible.
