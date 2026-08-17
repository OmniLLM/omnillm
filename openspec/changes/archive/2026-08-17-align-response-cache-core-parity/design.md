## Context

See `proposal.md` for motivation. OmniLLM currently hashes a selected canonical request projection, stores a canonical response JSON document inside a versioned Redis hash, and reserializes hits into Chat Completions or Anthropic Messages. The cache is operator-enabled, configuration is read live from SQLite, Redis failures are bounded and fail open, and streaming results are stored only after a complete terminal event. Responses requests bypass the cache today.

The requested parity changes request eligibility, public route coverage, canonical tool fidelity, and metering. The implementation must preserve the existing Redis key layout and canonical replay because cross-dialect and stream/non-stream reuse are existing contracts. It must also satisfy the repository's four-client, five-sequential-tool-call compatibility policy.

## Goals / Non-Goals

**Goals:**

- Reuse the existing canonical cache pipeline for all three generation dialects.
- Make exact-response identity complete for supported generation semantics and neutral to provider prompt-cache placement metadata.
- Preserve native Responses custom tools through storage, stream accumulation, and replay.
- Make served exact-response hits observable without conflating them with upstream provider prompt caching.
- Keep rollout backward compatible for Redis entries and existing positive TTL configuration.

**Non-Goals:**

- Byte-for-byte LiteLLM Redis compatibility or original HTTP/SSE replay.
- Semantic/vector caching, distributed affinity, miss coalescing, locks, rate limits, spend counters, or auth/config data in Redis.
- New exact-response settings UI, new public generation routes, embeddings caching, or a broader tenant/security redesign.
- Changing provider prompt-cache request forwarding, provider affinity, or provider cache-usage normalization.

## Decisions

### Preserve canonical response storage and Redis v1 layout

The cache will continue storing canonical responses at:

```text
<prefix>:response-cache:v1:entry:<sha256>
```

Existing Redis hash fields, Lua scripts, hit accounting, native TTL, namespace-scoped clear/statistics, circuit breaker, and recovery behavior remain unchanged. The JSON inside `response_data` gains optional custom-tool fields only, allowing older entries to decode as ordinary function calls.

Alternative considered: store complete per-dialect wire responses as LiteLLM does. This was rejected because it would fragment keys by route and stream mode, lose current cross-dialect reuse, and require a Redis schema migration without improving the requested observable behavior.

### Define exact-input TTL caching rather than deterministic-only caching

Eligibility will require a non-nil, valid canonical request on a supported generation route, but will not gate on sampling values. `temperature`, `top_p`, and other supported generation controls remain in the key, so only requests with identical supplied semantics can hit.

This intentionally permits replay of a prior stochastic result during the TTL, matching LiteLLM's core behavior. Opt-in enablement, short default lifetime, and existing bypass/off controls remain the operator safeguards.

Alternative considered: continue requiring `temperature: 0`. This was rejected because maintained coding-agent clients commonly omit sampling fields and the user explicitly selected core LiteLLM parity.

### Change only the fallback TTL

The effective default becomes 60 seconds. A positive value already persisted in SQLite remains authoritative, so upgrading does not rewrite operator intent or alter existing Redis entry expirations. Existing entries retain their write-time native TTL; only later writes use the new fallback when no valid positive configuration exists.

Alternative considered: migrate every stored TTL to 60 seconds. This was rejected because configured TTL is durable operator state and existing Redis TTLs cannot be safely reinterpreted.

### Use a reviewed, prompt-cache-neutral semantic projection

Key material will include all supported generation semantics: model, ordered system/messages, tools and custom format, tool choice, sampling controls, output limits, stops, response format, `PreviousResponseID`, forwarded user identity, and generation-affecting extensions such as thinking budget. It will exclude stream mode, headers, inbound API dialect, retry/fallback flags, and provider prompt-cache controls.

Provider cache controls will be stripped from deep-copied system blocks, content parts, and tool definitions, and top-level native cache key/retention policy will be excluded. This makes exact-response identity independent of where an upstream prefix is cached while leaving original directives intact for a miss.

If projection serialization fails, key construction returns no usable key and the request proceeds upstream without a cache operation. The current model-only fallback digest will be removed because it can collide across unrelated malformed internal requests.

Alternative considered: include inbound dialect or provider instance. This was rejected to preserve current canonical cross-dialect replay and pre-dispatch cache lookup. The short TTL and explicit clear operation bound provider-reconfiguration staleness.

### Include `PreviousResponseID` in identity

A Responses `previous_response_id` changes server-side conversation state. It will therefore participate in semantic identity instead of making all chained requests ineligible. Distinct chain identifiers cannot collide, while an exact repeat of the same chain state can hit.

Alternative considered: bypass all chained Responses requests. This was safer but unnecessarily narrower than LiteLLM parity and would exclude normal coding-agent continuation requests.

### Extend canonical cache fidelity for native custom tools

The cached tool projection will retain `ToolKind`, `RawInput` with presence, and `Namespace` in addition to existing ID, name, and arguments. The stream accumulator will track custom raw-input deltas separately from function argument deltas and reconstruct the correct canonical call. Synthetic replay will emit custom-input deltas for custom tools and JSON argument deltas for function tools.

Absent new fields decode as legacy function calls. No Redis schema-version bump is needed because the stored JSON evolves additively.

Alternative considered: disable caching for tool calls. This was rejected because LiteLLM caches tool-bearing responses and the project requires tool-loop compatibility; lossless canonical representation is the correct boundary fix.

### Integrate Responses through existing serializers

Responses lookup will occur after ingestion and canonical request preparation, matching Chat and Messages. On a hit, non-streaming output uses the existing Responses serializer. Streaming output uses the existing canonical stream synthesizer and Responses SSE converter. A hit header and hit metering record are committed only after conversion succeeds; conversion failure falls through to normal provider resolution.

Misses populate only after successful normalization and serialization. Streaming misses attach the existing accumulator and write only after a successful terminal event. Error, cancellation, or channel closure without a terminal event cannot populate.

Alternative considered: cache Responses wire objects/events. This was rejected because it would duplicate serialization logic and break canonical replay.

### Meter served hits as response-cache traffic

A narrow route helper will enqueue a normal request log only after a cache entry is successfully served. It will preserve aggregate input/output token counts as served-volume information, use a reserved source such as `response-cache`, and leave all provider cache-read/write fields nil. The derived provider prompt-cache status is therefore `unknown`.

Historical provider cache counters may remain inside the response payload for client response compatibility, but they are not copied into the new metering row because no provider call occurred.

Alternative considered: omit hit metering, as today. This was rejected because expanding cache coverage would make request totals undercount served traffic. Treating historical counters as current provider activity was rejected as misleading.

### Keep implementation scope bounded

Chat and Messages receive only the changes needed for safe key construction, conversion-committed hit accounting, and consistent logging. Admin endpoints, Redis configuration, provider adapters, affinity, and frontend surfaces remain unchanged.

## Risks / Trade-offs

- **[Risk] Stochastic output is replayed** → Keep caching opt-in, use a 60-second fallback, retain sampling values in identity, preserve bypass/off controls, and document the behavior.
- **[Risk] Cached tool calls can trigger repeated client-side side effects** → Treat this as exact-response replay semantics, preserve IDs and payloads exactly, and verify five-turn client loops; operators can disable or bypass caching for such traffic.
- **[Risk] Responses continuation IDs are reused** → Include `PreviousResponseID` in identity and verify distinct chain isolation. Cached provider response/call IDs intentionally remain stable as part of exact replay.
- **[Risk] Shape-specific behavior collides across dialects** → Keep only semantic CIF fields in identity, preserve native custom-tool kind, and test cross-dialect reuse only where canonical semantics are equivalent.
- **[Risk] Concurrent misses execute more than once** → Accept current best-effort last-writer behavior; miss coalescing is outside this parity scope.
- **[Risk] Redis payloads grow with native tool metadata** → Add only small optional fields; retain existing command deadlines and fail-open behavior.
- **[Risk] Hit metering can confuse provider analytics** → Use explicit response-cache attribution and nil provider cache detail; add database aggregation regressions.
- **[Risk] Synthetic streams differ from original chunk timing** → Preserve the existing semantic replay contract and assert one terminal event with no duplicate output rather than byte/timing equality.

## Migration Plan

1. Validate and approve this OpenSpec change before implementation.
2. Add failing deterministic regressions for eligibility, identity, custom tools, Responses replay, metering, and maintained client shapes.
3. Implement the additive canonical payload and route behavior without changing the Redis schema or adding a database migration.
4. Update documentation and run focused, real-Redis, four-client deterministic, bounded live-when-available, and full repository checks.
5. Deploy normally. Existing positive TTL settings and existing Redis entries remain valid; old cached tool calls decode as functions.
6. Roll back by reverting the code/config-default change. Entries containing additive JSON fields remain safe for older decoders because unknown JSON fields are ignored; operators can clear the namespaced cache if desired.
7. Archive the change only after every task and verification item passes.
