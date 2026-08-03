## Context

The full Go race suite currently fails in Alibaba provider tests and server-level Alibaba integrations. Failures cluster around two contracts: Qwen 3.6 Plus request construction no longer emits the `enable_thinking` value expected by tests, and live model discovery returns an identifier where tests expect a metadata-enriched name. Investigation must determine whether runtime behavior regressed or test expectations became stale before changing either side.

## Goals / Non-Goals

**Goals:**

- Establish one normative Qwen 3.6 Plus thinking-flag behavior across provider and server paths.
- Establish one normative model metadata enrichment behavior for live discovery.
- Restore the complete Go race suite as a reliable repository gate.

**Non-Goals:**

- Change MCP tool-argument normalization.
- Refactor unrelated Alibaba authentication, endpoint selection, or transport behavior.
- Make tests pass by weakening assertions without confirming upstream behavior.

## Decisions

### Make Qwen 3.6 Plus reasoning compatibility provider-owned

Qwen 3.6 Plus SHALL retain its verified `enable_thinking=true` behavior even when the optional `models.dev` metadata service is unavailable or lacks the model. Implement a narrow provider-owned known-model fallback while preserving explicit no-thinking exceptions for DeepSeek V4 Flash, Qwen 3.5 Plus, and GLM 5.1.

Alternative considered: continue failing closed when metadata is unavailable. Rejected because request behavior then changes with an unrelated external metadata lookup and contradicts the established provider/server contract.

### Keep model-discovery fallback and make tests deterministic

Live Alibaba discovery may enrich known model IDs when metadata is available and SHALL retain raw identifiers when metadata is unavailable. Tests will inject or seed a deterministic metadata service fixture to cover both known enrichment and unknown/raw fallback; production discovery fallback does not change.

Alternative considered: hard-code display metadata in production. Rejected because the metadata service is the intended enrichment source and raw-ID fallback is already safe.

### Keep request and model-discovery fixes separable

Treat thinking-flag behavior and model metadata enrichment as distinct implementation tasks with focused tests. They share a provider capability but need not share production changes.

### Verify at provider and server levels

Run focused Alibaba unit tests first, then server integrations that exercise Alibaba through public routes, and finally the repository-wide race suite. This catches divergence between adapter behavior and route-level integration.

## Risks / Trade-offs

- **[Risk] Live API behavior may differ by region or SDK version.** → Use recorded request/response fixtures or local test servers and avoid relying on external network state.
- **[Risk] Changing `enable_thinking` can affect response shape or latency.** → Cover plain chat, tool calls, and local re-streaming independently.
- **[Risk] Metadata enrichment can change client-visible model names.** → Preserve stable identifiers and constrain changes to documented display metadata.

## Migration Plan

No persistence migration is expected. Deploy provider/test corrections together. Roll back the isolated Alibaba changes if upstream compatibility regresses.
