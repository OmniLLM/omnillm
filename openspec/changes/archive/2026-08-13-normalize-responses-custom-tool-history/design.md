## Context

See `proposal.md` for motivation. Responses ingestion currently maps `function_call` and `function_call_output` into the existing CIF tool vocabulary, but rejects custom-tool items before routing. CIF stores tool arguments as an object and tool-result content as a string; provider builders and public serializers emit ordinary function-tool wire items. The observed Droid history originated from `ApplyPatch` arguments shaped as `{"input":"{}"}` and was replayed as a custom call whose raw `input` was `"{}"`.

## Goals / Non-Goals

**Goals:**

- Unblock Droid custom-tool continuation requests without changing CIF types or provider adapters.
- Preserve raw custom input, output content, ordering, and call association within the existing canonical vocabulary.
- Distinguish omitted required fields from explicitly present empty values.
- Fail malformed custom items before provider dispatch with the existing structured Responses error path.

**Non-Goals:**

- Native or lossless retention of custom/function wire identity.
- Retention of custom item `id`, `status`, `namespace`, or tool `format`.
- Native custom-tool emission by providers, public Responses serialization, or SSE.
- Cross-dialect preservation of grammar-constrained custom-tool semantics.

## Decisions

### Normalize raw custom input under one canonical `input` argument

A `custom_tool_call` becomes an existing canonical tool call with `ToolArguments` equal to `{"input": rawInput}`. The raw string is never passed through function argument JSON parsing.

This reproduces the canonical shape that generated the observed Droid `ApplyPatch` invocation and gives existing function-tool provider builders a valid object. Using `_unparsable_arguments` was rejected because it changes the tool's declared argument name; parsing raw input as JSON was rejected because `"{}"` would incorrectly become an empty object and arbitrary patch programs are not JSON.

### Preserve missing-versus-empty field semantics during decoding

Inbound item decoding records whether `input` and `output` were present independently from their values. Custom calls require an explicitly present string input, and custom outputs require an explicitly present supported output value. Empty strings and empty lists remain valid.

This avoids treating valid empty values as omitted while retaining actionable invalid-request failures.

### Encode content-list output as compact JSON tool-result text

String output maps directly to canonical result content. A list is validated against the supported Responses text/image/file output-content shapes and then compactly JSON encoded as a whole, preserving member order and fields. Scalars, objects, null, and unknown list member types are rejected.

Flattening a list to text was rejected because it discards image/file data and ordering. Extending CIF with a union was rejected for this focused change because it would require all serializers and providers to understand a new representation.

### Normalize custom definitions into function-tool schemas

A named top-level tool with `type: "custom"` becomes an existing canonical tool with the same name and description and this schema:

```json
{
  "type": "object",
  "properties": {"input": {"type": "string"}},
  "required": ["input"],
  "additionalProperties": false
}
```

This aligns declared tool arguments with normalized history and allows all existing provider strategies to consume the request. Custom `format` is intentionally discarded.

### Require explicit custom item discriminators

Only `type: "custom_tool_call"` and `type: "custom_tool_call_output"` activate custom normalization. Existing missing-type inference remains limited to legacy message and function items because an `input` field alone is ambiguous.

### Test with sanitized fixtures, not payload-bearing logs

The route regression uses an in-process synthetic request and captured canonical provider request. Production trace logging remains unchanged because provider payload traces can expose prompts, tool inputs, and results.

## Risks / Trade-offs

- **Custom wire identity is lost after ingestion** → Document normalization explicitly and keep native custom-tool round-trip support as a separate change.
- **Grammar-constrained custom formats become a plain string contract** → Limit this fix to compatibility with clients such as Droid whose downstream tool schema accepts one raw input string.
- **Content-list output becomes JSON text for downstream models** → Preserve the complete ordered representation and cover it in tests rather than silently flattening or dropping members.
- **Provider behavior may differ from native custom tools** → Maintain the narrow acceptance objective and verify the observed Droid continuation plus unchanged function-tool histories.

## Migration Plan

1. Add and validate parser and route regressions using sanitized history.
2. Deploy as a backward-compatible ingestion expansion; no persistence or configuration migration is required.
3. Roll back the commit if provider behavior regresses. Existing requests remain wire-compatible because no stored data or public output shape changes.
