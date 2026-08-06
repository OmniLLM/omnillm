## Why

Native upstream model identifiers may contain `/`, but OmniLLM currently interprets the first segment of every slash-containing identifier as a provider qualifier. This prevents models such as Alibaba's `kimi/kimi-k3` from routing by their full identifier and can affect any provider exposing organization- or namespace-qualified model IDs.

## What Changes

- Resolve an advertised complete slash-containing model identifier through ordinary provider selection before considering its first segment as a provider qualifier.
- When the complete identifier is unavailable, treat its leading segment as a provider qualifier only when that segment resolves to a known provider instance identifier or subtitle.
- Preserve internal slashes when an explicitly provider-qualified model is reduced to its upstream model identifier.
- Stop Alibaba model remapping from stripping native namespace segments.
- Apply the same provider qualification and native slash-preservation behavior to chat completions, Anthropic Messages, and Responses requests.
- Retain normal provider selection and failover for unqualified native model identifiers; retain exact single-provider pinning for recognized qualifiers.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `routing-failover`: Disambiguate recognized provider qualifiers from native slash-containing model identifiers and preserve the complete upstream identifier during dispatch.
- `providers`: Require Alibaba compatibility transforms to preserve namespaced upstream model identifiers.
- `gateway-api`: Require consistent provider qualification and native model identifier handling across supported generation API dialects.

## Impact

- Affected code: shared model resolution and candidate construction, Alibaba model remapping, and the Responses routing path.
- Affected APIs: model selection semantics for chat completions, Anthropic Messages, and Responses requests.
- Compatibility: a slash-containing model advertised by an active provider now takes precedence as a complete native identifier, including when its first segment collides with a provider instance or subtitle. If the complete identifier is unavailable, recognized provider instance identifiers and subtitles retain provider-pinning behavior; otherwise resolution fails through the ordinary full-identifier path.
- Tests: routing unit tests, Alibaba adapter tests, shared multi-provider integration tests, and ingress-specific provider-prefix regressions.
- Dependencies and persistence: no dependency, schema, or stored-data changes.
