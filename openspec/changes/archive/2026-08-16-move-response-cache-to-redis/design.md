## Context

See `proposal.md` for motivation. The current exact-response cache is reached from Chat Completions and Anthropic Messages after CIF ingestion and before provider resolution. `internal/lib/responsecache` already owns deterministic eligibility, the semantic SHA-256 key, canonical response encoding, bypass controls, stream accumulation, and replay, but delegates payload persistence directly to a process-global SQLite store. The same single-connection SQLite database remains required for durable configuration and all non-response-cache state.

The response cache is disabled by default. Its enabled flag and TTL are read from SQLite per request, while cache failures are already intended to be best-effort. Redis is not currently a dependency, bundled service, or CI prerequisite; desktop and container launches must continue to work without it. Redis connection URLs can contain credentials and therefore require redaction. Existing migration 15 created the SQLite cache table and cannot be rewritten.

## Goals / Non-Goals

**Goals:**

- Isolate the latency-sensitive response-cache data plane behind a context-aware store owned by the response-cache package.
- Use bounded, recoverable Redis access while keeping all model-serving paths fail-open.
- Preserve the storage-independent CIF keying, encoding, replay, route, and header contracts.
- Keep administrative statistics and clearing namespace-safe and off the inference hot path.
- Make startup and shutdown resource cleanup correct on every return path.

**Non-Goals:**

- Moving provider prompt caching, prompt-cache metering, provider model catalogs, runtime settings, or any durable record to Redis.
- Falling back to, dual-writing, or migrating response payloads from SQLite.
- Bundling or automatically managing a Redis/Valkey daemon.
- Making Redis part of gateway liveness or readiness.
- Adding response caching to Responses or embedding routes.

## Decisions

### 1. The response-cache package owns a narrow storage contract

Introduce a `Store` interface and record/stat types in `internal/lib/responsecache`, with context-aware `Get`, `Save`, `Stats`, `Clear`, availability, and `Close` operations. A concurrency-safe package configuration seam supplies the active store to request and admin paths; the default unavailable store returns misses/no-op writes rather than touching SQLite.

This preserves the existing low-disruption package-level usage pattern while removing the database package dependency from cache reads and writes. Explicit store injection through every route constructor was considered, but it would broaden unrelated handler signatures and make the change harder to isolate. Direct use of the Redis client from routes was rejected because it would couple protocol, lifecycle, and outage policy to HTTP handlers.

Request cache calls will receive `c.Request.Context()`. Startup, recovery, and admin operations will use their own bounded contexts. Canonical `Key`, `Cacheable`, `encodeResponse`, `decodeResponse`, `StreamAccumulator`, and replay behavior remain storage-independent.

### 2. Use the official Redis client with URL-based configuration

Use `github.com/redis/go-redis/v9` and parse `redis://` or `rediss://` URLs with the library's URL parser so username, password, database number, and TLS are consistently supported. Resolution order is an explicit startup flag, then `OMNILLM_RESPONSE_CACHE_REDIS_URL`, then `redis://127.0.0.1:6379/0`. A separate flag/environment value controls a validated key prefix whose default is `omnillm`.

`RunServer` creates the client after SQLite initialization, configures short dial/read/write and pool-wait bounds, disables multiplicative retries, installs the store, and performs one bounded ping. Parsing, authentication, or connectivity failure logs a redacted warning but does not abort startup. The store retains a recoverable client when parsing succeeded, even if the initial ping failed. Cleanup is registered immediately with `defer`, alongside refactored database worker/database cleanup, so listener failures and signal shutdown close resources exactly once.

A local default was chosen over requiring explicit configuration because the requested deployment is a local Redis server. A mandatory startup dependency was rejected because it would break existing desktop, container, and zero-service installs. An empty URL meaning "disabled" was considered, but cache enablement already has a durable explicit switch; connection availability is reported separately.

### 3. Versioned, namespaced Redis hashes retain the canonical envelope

Each entry uses:

```text
<prefix>:response-cache:v1:entry:<64-character semantic digest>
```

The key never includes prompts, model names, tool arguments, or response text. The Redis value is a hash containing `response_data`, `model_id`, `created_at_ms`, `hit_count`, `last_hit_at_ms`, and `schema_version`. The existing canonical response JSON remains byte-for-byte compatible.

A Lua save operation replaces all fields, resets hits, records creation time, and applies a positive native expiration atomically. A Lua get operation reads the payload and atomically increments hit count and updates last-hit time without removing expiration. Missing or malformed entries are misses; corrupt entries are deleted best-effort. Model metadata remains for diagnostics even though replay uses the canonical payload.

A single JSON string value plus separate counters was considered, but would require more commands or rewriting the full response on every hit. Separate payload and metadata keys were rejected because expiry and replacement could diverge. Versioned hash keys allow future envelope changes to cold-start safely.

### 4. Native TTL is assigned when an entry is written

The live SQLite setting is still loaded per request. A positive effective TTL is passed to `Save`, which sets native Redis expiration. New and refreshed entries therefore observe settings without restart. Existing entries retain their write-time expiration after an operator changes the TTL; an expired key cannot be resurrected.

This intentionally replaces SQLite's read-time age check, where increasing TTL could make an unpurged old row readable again. Clearing all entries on every TTL edit was considered, but creates surprising data loss and turns a setting update into an availability-sensitive Redis administration operation. Updating every key's expiration was rejected as an O(N) control-plane action. Write-time native TTL gives Redis autonomous reclamation and predictable request cost. A missing, zero, negative, or invalid persisted TTL continues to normalize to one hour; the admin API continues rejecting negative input.

The unused production `PurgeExpired` operation is removed because native Redis expiry performs reclamation.

### 5. Fail-open behavior uses a small recovery circuit

All Redis commands are bounded. A backend command failure marks the store unavailable and opens a concurrency-safe circuit for a cooldown. While open, request reads return immediate misses and writes become immediate no-ops rather than paying a timeout on every request. After cooldown, exactly one half-open probe is allowed; success restores normal operations and failure reopens the circuit. A bounded background probe also permits recovery during periods without cacheable traffic. Only availability transitions are logged.

A read error is indistinguishable from a miss to model-serving code; a write error is swallowed after logging because the upstream response has already succeeded. There is no SQLite fallback. Admin settings reads return durable configuration plus neutral statistics and degraded availability on Redis failure. Clear remains explicit and returns an error if Redis is unavailable.

Relying solely on go-redis connection retries was rejected because sustained outages would add latency to every eligible request. Permanently disabling the store after a failed startup ping was rejected because recovery would require restarting OmniLLM.

### 6. Statistics and clear are bounded and namespace-scoped

`Stats` uses cursor-based `SCAN` over the exact versioned entry pattern, followed by batched/pipelined hit-count reads. This yields live entry and hit totals without global counters drifting as keys expire. It runs only on authenticated administrative paths with its own deadline.

`Clear` uses the same bounded scan and batched `UNLINK` (falling back to `DEL` only when unsupported), returning the number actually removed. It never uses `KEYS` or `FLUSHDB`. A store-level read/write lock serializes local saves against clear completion; other Redis users and other namespaces are unaffected. Concurrent writes from a separate OmniLLM process can still race a scan, so complete cross-process clearing is defined over the entries discovered during that operation. A generation-pointer O(1) logical clear was considered, but it complicates statistics and physical reclamation and would leave old namespaces consuming memory until an additional sweeper runs.

Global counters were rejected because native expiry would make totals inaccurate without keyspace notifications or extra expiry bookkeeping.

### 7. Retire only the SQLite response-cache schema

Fresh database creation no longer creates `response_cache` or its index. Historical migration 15 remains unchanged. A new forward migration drops the index/table for upgraded databases. No rows are copied because cached generations are disposable and may already be stale; the first upgraded run starts cold. Cache enabled/TTL keys remain in the SQLite config table, and all other caches and durable data are unchanged.

Leaving the table indefinitely was considered but would imply a possible fallback and retain unneeded response content on disk. Migrating rows was rejected because migration execution must not depend on Redis availability and because native TTL cannot be reconstructed reliably from the active setting alone.

### 8. Operator surfaces remain compatible and credential-safe

The response-cache settings endpoint retains `enabled`, `ttl_seconds`, `entries`, and `total_hits` and adds `backend: "redis"` and `available`. A failed stats read still returns HTTP 200 with settings, zero totals, and `available: false`; clear failures remain non-success responses. CLI settings and doctor output show backend availability without printing the Redis URL. Existing enable, disable, TTL, and clear commands remain compatible.

`/`, `/health`, and `/healthz` remain liveness-only and return success regardless of Redis state. Docker and desktop continue launching the gateway without Redis; documentation explains that container-local `127.0.0.1` is not a sibling Redis service and shows explicit service-host configuration.

## Risks / Trade-offs

- **[Native expiry changes TTL-extension behavior]** → Specify that expiration is assigned at write time, test non-resurrection, and document the cold/ephemeral nature of entries.
- **[Redis outage adds latency or log floods]** → Use short deadlines, disabled retries, circuit bypass, transition-only logging, and bounded probes.
- **[Clear or stats are O(N)]** → Keep them authenticated and off the request path, scan in bounded batches, pipeline metadata reads, and enforce admin deadlines.
- **[A cross-process write can race clear]** → Guarantee local save/clear ordering and report actual removals; document that distributed clear covers entries observed by its cursor scan.
- **[Credentials leak through URL errors]** → Parse once, emit only redacted endpoint metadata, avoid returning connection strings in APIs, and test logs/output with credential-bearing URLs.
- **[Lua behavior differs in test emulators]** → Use an in-process Redis-compatible test server for normal CI and provide an opt-in real-Redis integration test for script and command parity.
- **[Package-global store replacement races tests or shutdown]** → Guard configuration/access, make close idempotent, restore test stores, and run concurrency suites under the race detector.
- **[Dropping the table removes cached user content permanently]** → Treat it as disposable by contract, preserve all configuration and durable records, and test the migration boundary.
- **[Current comments misstate tool/stream eligibility]** → Align documentation with the existing normative caching spec without changing eligibility in this refactor.

## Migration Plan

1. Land the new Redis dependency, store abstraction, Redis implementation, and hermetic tests while preserving the existing external cache contract.
2. Wire startup configuration, bounded ping/recovery, and unconditional resource cleanup; validate startup with and without Redis.
3. Switch request and admin callers to the configured response-cache store and add backend availability fields.
4. Append the forward SQLite migration, remove fresh-schema creation and the SQLite store, and verify unrelated records survive upgrade.
5. Update operator documentation and run deterministic route/client compatibility, race, build, and optional real-Redis smoke tests.
6. Deploy with Redis available where caching is desired; otherwise the cache remains degraded/fail-open and model serving is unchanged.

Rollback to a pre-change binary is operationally safe for durable records, but the new migration will have removed legacy cached rows. The older binary recreates the response-cache table through its historical `CREATE TABLE IF NOT EXISTS` startup path and begins with a cold SQLite cache. Redis keys remain isolated and can expire or be cleared independently.
