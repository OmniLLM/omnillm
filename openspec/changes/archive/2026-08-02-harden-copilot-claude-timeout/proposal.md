## Why

Seven GitHub Copilot `claude-opus-5` Anthropic requests failed between 17:29 and 17:42 on 2026-08-02 after almost exactly 120 seconds waiting for response headers. Copilot Claude streams intentionally use native streaming for tool-call fidelity, but their initial header wait currently uses the ordinary non-streaming chat budget when the route attempts failover, making slow first-token reasoning requests repeatedly terminate as HTTP 502.

## What Changes

- Give Copilot Claude chat-completions requests a dedicated configurable header-wait budget larger than the ordinary chat/models/embeddings budget.
- Preserve the no-fixed-timeout stream client after response headers arrive.
- Add timeout failure logs with provider, endpoint, model, configured budget, elapsed time, and request identifier when available.
- Add regression tests proving Claude uses the larger budget while ordinary chat, model-list, and embedding calls retain their current timeout.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `providers`: Copilot Claude chat-completions calls receive a reasoning-capable initial response-header budget and actionable timeout diagnostics.

## Impact

- Affects `internal/providers/copilot` client selection and request logging.
- Reuses the existing shared timeout/transport utilities and introduces a Copilot-specific environment override for Claude chat-completions.
- Does not change API shapes, streaming event serialization, authentication retry, provider failover ordering, or timeouts for non-Claude Copilot operations.
