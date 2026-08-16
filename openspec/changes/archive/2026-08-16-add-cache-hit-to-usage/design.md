## Context

Provider prompt-cache data is persisted in nullable request-log columns after PR #182, and public model response dialects expose provider cache counters. The authenticated metering endpoints, CLI usage command, and browser Metering page still project only aggregate input/output totals. The presentation layer therefore lacks the distinction between an explicitly reported zero cache read and a provider that did not report cache usage.

## Goals / Non-Goals

**Goals:**

- Define one derived status contract shared by database queries, admin API, CLI, and UI.
- Preserve nullability so `unknown` is never collapsed into `miss`.
- Add cache token and status aggregates to existing filtered usage views.
- Keep existing token totals and raw JSON output backward compatible.
- Make provider prompt caching visibly distinct from the exact-response cache.

**Non-Goals:**

- Change provider prompt-cache request behavior or usage parsing.
- Add a new database migration; nullable counters already exist.
- Infer monetary savings or cache write/read pricing.
- Treat affinity selection as cache-hit evidence.
- Add exact-response-cache hit accounting to provider prompt-cache status.

## Decisions

### Derive status from nullable read tokens

Use the persisted nullable cache-read counter as the only provider prompt-cache status source:

- non-null and greater than zero → `hit`
- non-null and equal to zero → `miss`
- null → `unknown`

This can be expressed consistently in Go and SQL and requires no new stored column. Persisting a redundant status was rejected because it can drift from the token value and would require a migration. Treating null as miss was rejected because many providers omit cache details even when caching is implicit.

### Compute filtered aggregates in SQL

Extend the existing metering statistics and grouping queries with `SUM` expressions over nullable cache token columns and `CASE` expressions for status counts. Every endpoint continues to use the existing model/provider/client/API-shape/time filter construction; an optional status filter adds a predicate over the same derivation.

Application-side aggregation was rejected because it would require loading all matching rows, break pagination expectations, and duplicate SQL grouping behavior.

### Add fields without changing existing totals

Raw records gain `prompt_cache_status`; the existing nullable token fields remain unchanged. Summary and grouped result types gain cache read/write totals and hit/miss/unknown counts. Existing `input_tokens`, `output_tokens`, and `total_tokens` retain their current definitions, so old consumers can ignore the new JSON fields.

### Present detailed rows and compact summaries

The Usage page adds summary cards or a compact cache summary for hit rate and cache token volume, and request-log rows add status plus cache read/write detail. Grouped model/provider/client tables include cache status counts or hit rate without removing current columns. On narrow screens, cache detail may be grouped into one cell or horizontal-scroll region, but request identity, totals, and status remain inspectable.

The CLI follows the same information hierarchy: cache summary near total usage and concise cache columns in breakdown output. JSON mode remains pass-through.

### Keep response-cache semantics separate

Labels use “Provider prompt cache” and define unknown explicitly. The UI and CLI do not infer exact-response-cache hits from provider counters, affinity, or latency. A future response-cache metering change can add a separate status field without changing this vocabulary.

## Risks / Trade-offs

- **[Risk] Wider tables reduce readability** → Group related cache fields, use concise headings/tooltips, and verify narrow viewport behavior.
- **[Risk] SQL status expressions drift across queries** → Centralize the status SQL expression and scan helpers used by raw, summary, and grouped queries.
- **[Risk] Older clients assume fixed JSON shapes** → Add fields only; do not rename or reinterpret existing fields.
- **[Risk] Hit rate denominator is misleading** → Report hit/miss/unknown counts and compute hit rate only over known statuses, with the denominator documented.
- **[Risk] Null handling differs between Go and TypeScript** → Use nullable Go/JSON fields and optional-null TypeScript types with explicit unit tests for null, zero, and positive values.

## Migration Plan

1. Validate and approve this OpenSpec change.
2. Add backend query/type tests for raw status, aggregates, filters, and null handling.
3. Extend metering API types/queries, then CLI output and frontend API types/UI.
4. Run backend, frontend, CLI, accessibility/responsive, and full repository checks.
5. Archive only after all tasks and checks pass; rollback removes additive query/output fields and UI columns without database changes.
