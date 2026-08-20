## Context

See `proposal.md` for motivation. OmniLLM currently keeps exact-response entries in an operator-scoped Redis namespace. Each entry stores canonical response JSON and a per-entry hit count; the settings endpoint scans live entries to return `entries` and the compatibility `total_hits` aggregate. Missing lookups are not counted, and no payload-size statistic exists. Provider prompt-cache usage is separately persisted in SQLite and shown by `omnillm usage`.

The change crosses the Redis store, authenticated admin API, and Cobra CLI, and it extends a public JSON response. Redis must remain optional and fail open, statistics must remain namespace-isolated, and no Redis credentials, prompts, or response contents may be exposed.

## Goals / Non-Goals

**Goals:**

- Make local exact-response cache footprint and lookup effectiveness observable.
- Preserve existing API fields and the behavior of `settings get response-cache`.
- Give hit/miss counters well-defined reset, outage, and denominator semantics.
- Keep collection bounded and avoid adding request-path SQLite writes.

**Non-Goals:**

- Measuring Redis allocator overhead or total database memory.
- Reclassifying successfully served responses or provider prompt-cache metering.
- Persisting response-cache counters across a namespace clear or Redis data loss.
- Adding cache entry listing, per-model breakdowns, or a console UI.

## Decisions

### Add `cache stats` while reusing the settings endpoint

Register a new root `cache` command with a `stats` child in the Admin group. The command will call the existing authenticated `GET /api/admin/settings/response-cache` endpoint. Table mode selects and formats the statistics fields; JSON mode uses the existing raw-response path.

This keeps one response-cache status contract and makes `cache stats` immediately compatible with the global server, API-key, and output flags. `settings get response-cache` remains available and unchanged as a settings-oriented view. A standalone `/stats` endpoint was rejected because it would duplicate settings/backend context and create needless client/API surface.

### Keep compatibility and lookup counters separate

Retain `total_hits` as the sum of per-entry hit counts across currently live entries. Add `lookup_hits` and `lookup_misses` as namespace-level counters for the current statistics window. This avoids silently changing an existing field whose value naturally falls when entries expire or are replaced.

A lookup hit is counted when Redis returns a structurally usable namespaced cache entry, before API-dialect conversion. A lookup miss is counted when Redis successfully establishes that no usable entry exists, including absence, expiration, or malformed entry cleanup. Disabled/ineligible bypasses and Redis failures do not alter either counter. The names use `lookup` rather than `served` because a retrieved entry can still fail downstream conversion and fall through to a provider.

Counting only successfully served hits was rejected because lookup is centralized in the cache store while replay conversion occurs in multiple generation routes; coupling counter updates to every output path would be less atomic and could drift across API shapes.

### Store aggregate counters in the same Redis namespace

Use namespace metadata keys for lookup hits, lookup misses, and the statistics-window start. Update the applicable counter in the same Redis-side operation that resolves the lookup outcome so concurrent requests cannot lose increments. Metadata keys are excluded from entry scans and are removed by namespace clear.

The statistics-window start is initialized with the first successfully counted lookup and cleared with the namespace. It is null when neither counter has observations. Redis-native counters were chosen over process-local atomics so statistics remain coherent across multiple OmniLLM processes sharing the namespace and survive a gateway restart. SQLite persistence was rejected because cache payloads and counters are intentionally owned by Redis and request-path SQLite writes would add contention.

### Report logical canonical payload bytes

Extend the existing namespace statistics scan to aggregate the encoded byte length of each live entry's canonical response data. Return this as `payload_bytes`. It is deterministic, directly tied to OmniLLM-managed response data, and portable across supported Redis-compatible backends.

Redis `MEMORY USAGE` was rejected because allocator overhead varies by server implementation/version and would make tests and operator interpretation unstable. The metric therefore does not claim to represent total Redis memory consumption.

### Derive hit rate and degrade neutrally

Return `lookup_hit_rate` as a JSON number in the inclusive range 0–1 when the denominator is nonzero; otherwise return null. Return `stats_since` as an RFC 3339 UTC timestamp when observations exist; otherwise null. Compute the rate from a consistent statistics read rather than storing a floating-point value.

If Redis statistics cannot be read, preserve HTTP 200 and durable configuration fields, set `available` false, return zero for numeric statistics, and return null for rate/window fields. This matches the current fail-open settings behavior and lets `cache stats` remain useful during an outage.

## Risks / Trade-offs

- **[Statistics scans grow with live entry count]** → Reuse the bounded namespace scan and pipeline payload-length/hit reads in batches; do not issue one network round trip per entry.
- **[A hit can later fail dialect conversion]** → Name and document the metric as a lookup hit; keep successfully served response-cache traffic in existing metering.
- **[Malformed entries affect miss counts]** → Treat an unusable entry as a miss because upstream execution is required, while retaining diagnostic logging and best-effort cleanup.
- **[Counters can disappear through Redis eviction or operator data deletion]** → Define a current Redis-backed statistics window and expose `stats_since`; do not imply lifetime durability.
- **[Older servers omit additive fields]** → Decode missing numeric fields as zero and nullable fields as unavailable in the new CLI renderer; preserve raw JSON behavior.

## Migration Plan

1. Deploy the additive Redis store and API fields; existing clients continue using unchanged fields.
2. Deploy the CLI command, which tolerates omitted additive fields when pointed at an older server.
3. Rollback requires no data migration: older binaries ignore namespace metadata keys, and a normal cache clear removes them through namespace-scoped cleanup.
