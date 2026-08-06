## Context

See `proposal.md` for motivation. Model resolution currently parses every first slash as provider qualification before consulting the provider registry. This makes native namespace syntax ambiguous. Chat completions and Anthropic Messages retain resolved attempts, while the Responses path collapses resolution to model strings and loses provider pinning. Candidate construction then invokes provider remapping; Alibaba currently removes everything through the last slash, which destroys native namespace segments even after shared resolution.

The change crosses shared routing, a provider adapter, and three public generation dialects. It must preserve existing provider-instance and subtitle qualification while allowing native slash-containing identifiers from Alibaba, ModelScope, OpenAI-compatible endpoints, and future providers.

## Goals / Non-Goals

**Goals:**

- Give an advertised complete native model identifier precedence over provider qualification.
- Fall back to provider qualification only when complete-identifier resolution is unavailable and the first segment is recognized.
- Use one attempt-oriented routing contract for chat completions, Messages, and Responses.
- Keep provider-specific remapping from repeating shared qualifier removal.
- Cover both unqualified native identifiers and explicitly provider-qualified native identifiers.

**Non-Goals:**

- Introduce a new provider-qualification delimiter or escaping syntax.
- Change provider priority, balancing, affinity, virtual-model, or retry policies.
- Change model discovery, persistence, or administrative model identifiers.
- Guarantee success for an unknown full model identifier that no active provider advertises.

## Decisions

### Resolve the complete identifier before classifying a prefix

For any slash-containing request, first build and evaluate the ordinary attempt using the complete model identifier. If one or more active providers advertise that exact identifier, use the existing priority, affinity, and failover behavior and do not interpret the first segment as qualification.

Only when complete-identifier resolution yields no candidates should routing split at the first slash, resolve the leading segment against provider instance identifiers first and subtitles second, and add a provider-pinned fallback attempt. This ordered-attempt approach preserves explicit qualification such as `alibaba-instance/kimi/kimi-k3` because the complete qualified string is normally not advertised, while allowing `kimi/kimi-k3` to remain native even when a provider or subtitle named `kimi` exists.

This chooses model-catalog evidence over syntax or registry-name precedence. A new delimiter or escape convention would be unambiguous but breaking. Registry-first classification was rejected after a live test demonstrated a real `kimi` provider collision.

### Preserve the entire suffix in the provider fallback

When the complete model attempt is unavailable and qualification succeeds, remove only the first segment and its delimiter. The remainder is opaque provider model text, so `alibaba-instance/kimi/kimi-k3` becomes `kimi/kimi-k3`, not `kimi-k3`.

The alternative of repeatedly taking the last segment is incompatible with namespace-qualified upstream catalogs.

### Make shared routing the sole qualifier-removal layer

Alibaba remapping will normalize only what its upstream contract requires, without stripping slash segments. Provider remapping remains available for actual aliases and compatibility transforms, but it must not infer gateway provider qualification from model text.

This avoids duplicated parsing rules whose behavior diverges between shared dispatch and adapters.

### Preserve resolved attempts through Responses dispatch

Responses will consume the same resolved attempt representation used by chat completions and Messages, including `ProviderID`, canonical/upstream model identity, and ordered alternatives. It will no longer reduce resolution to a pair of strings before candidate construction.

A Responses-only patch that carries one extra provider field was rejected because it would continue to bypass shared virtual-model and failover semantics and leave future routing drift likely.

### Treat unresolved leading segments as native model namespaces

An unknown first segment never creates a provider-pinned fallback. The full identifier follows ordinary model resolution and fails normally when no provider advertises it. A recognized first segment creates a fallback only after the complete identifier has no candidates.

## Risks / Trade-offs

- [A misspelled provider qualifier is interpreted as part of a native model identifier] → Ordinary model resolution returns not found when no provider exposes that full identifier; no provider fallback is added for an unrecognized prefix.
- [A provider instance or subtitle collides with the first namespace segment of a native identifier] → Exact full-model availability takes precedence; recognized qualification remains available as a fallback when the complete string is not advertised.
- [Responses refactoring changes more than slash handling] → Reuse the existing attempt pipeline and add parity tests for provider pinning, ordering, and retryable failover.
- [Provider remappers may contain similar slash stripping] → Audit all provider remappers during implementation and add a shared regression with a non-Alibaba namespaced model; change only destructive qualifier inference.

## Migration Plan

1. Add unit tests for registry-aware qualification and opaque suffix preservation.
2. Update shared resolution and make Responses retain resolved attempts.
3. Remove destructive slash stripping from Alibaba remapping and audit other adapters.
4. Add cross-dialect and multi-provider integration regressions.
5. Run the complete routing/provider tests and project verification suite before archive.

No data migration or staged rollout is required. Rollback consists of reverting the routing and adapter changes together; no persisted state changes are involved.
