## Why

OmniLLM supports three inbound generation API shapes and ten provider types, but compatibility coverage is fragmented and does not systematically prove that each client shape reaches every compatible model execution path or survives realistic multi-turn agentic tool use. A layered deterministic matrix plus an explicit, isolated live matrix will make cross-shape and cross-model regressions visible without making credentialed network tests mandatory in normal CI.

## What Changes

- Add a shared semantic compatibility fixture set for plain generation, streaming, sequential and parallel tool calls, long tool results, tool errors, and multi-turn agentic loops.
- Expand deterministic tests across OpenAI Chat Completions, Anthropic Messages, and OpenAI Responses request/response shapes.
- Cover each supported provider execution strategy and representative compatible model path, including providers that dynamically select Chat Completions or Responses upstream.
- Verify model resolution/remapping, upstream endpoint selection, streaming strategy, tool-call identity and argument accumulation, cancellation, timeout, and failover invariants.
- Add an opt-in live model matrix that runs each configured compatible API-shape/model combination using isolated temporary state and explicit credential references.
- Report live rows as pass, fail, skipped, or not applicable; missing credentials skip clearly, while supplied credentials that fail authentication or execution fail the row.
- Add smoke and extended live modes, with extended mode covering multiple tool cycles, parallel tools, large results, long-lived streams, and cancellation.
- Keep credentialed live tests outside mandatory CI and avoid exact natural-language output assertions.
- No production API, provider, routing, model, credential, or persistence behavior changes.

## Capabilities

### New Capabilities

- `model-compatibility-testing`: Defines deterministic and credential-gated live evidence that supported API shapes, providers, and compatible models preserve generation and agentic tool-use contracts.

### Modified Capabilities

None.

## Impact

- Adds reusable test fixtures and assertions under Go test-only packages.
- Expands ingestion, serialization, route, provider-contract, and server integration tests.
- Adds an isolated Bun/TypeScript live matrix runner, example manifest, package commands, and machine-readable result reporting.
- May update CI commands to run deterministic matrix tests; live network tests remain opt-in.
- Uses temporary runtime state for live testing and does not read or mutate the normal OmniLLM database unless explicitly configured.
