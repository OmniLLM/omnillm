## Why

Models can emit empty strings for optional MCP tool parameters, but some MCP servers validate a present empty value as invalid even though omitting the parameter is accepted. This causes otherwise valid tool calls, such as Slack searches without a channel context, to fail before execution.

## What Changes

- Normalize model-emitted tool arguments against the declared tool input schema before client-facing emission.
- Omit empty-string object members only when the corresponding property is not required by the declared schema.
- Preserve required empty strings, non-string values, nested values, and behavior when no usable schema is available.
- Preserve the identity, ordering, and independent accumulation of interleaved streamed tool calls.
- Add regression coverage for optional and required empty strings, interleaved streams, and cache replay.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cif-translation`: Refine tool-call fidelity so emitted arguments omit unset optional string properties according to the declared tool schema without changing required values or interleaved-call identity.

## Impact

- Affects canonical tool metadata lookup and tool-argument response/stream translation.
- Affects provider response paths that emit model-generated tool arguments, including Copilot chat-completions and Responses streams.
- Affects Anthropic-compatible response serialization and cached stream replay.
- Does not change public request schemas, dependencies, or provider-specific Slack behavior.
