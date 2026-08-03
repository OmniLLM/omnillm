## Why

The repository-wide Go race suite is currently red because Alibaba provider and server integration tests disagree with current Qwen 3.6 request flags and live model metadata enrichment. These baseline failures obscure regressions in otherwise unrelated changes and must be resolved independently from MCP argument normalization.

## What Changes

- Restore agreement between Alibaba Qwen 3.6 Plus request behavior and its provider/integration tests, including the `enable_thinking` contract.
- Restore consistent metadata enrichment for live Alibaba model discovery.
- Add or update focused regressions so the full Go race suite is a reliable gate again.
- Keep this work independent of `omit-empty-optional-mcp-arguments`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providers`: Clarify Alibaba Qwen request compatibility and model-discovery metadata behavior required by the provider baseline.

## Impact

- Affects Alibaba request construction, model discovery, provider tests, and server-level Alibaba integration tests.
- May update stale tests instead of runtime behavior where investigation proves the current implementation is normative.
- Does not affect MCP argument normalization, CIF tool-call streaming, or other providers.
