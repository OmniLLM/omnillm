## Why

The exact-match response cache currently shares OmniLLM's single-connection SQLite database with durable configuration and metering writes, adding disk-backed serialization and contention to a latency-sensitive, concurrent lookup path. Moving disposable response payloads to an optional local Redis service provides low-latency in-memory access while keeping model serving available when that acceleration service is absent or unhealthy.

## What Changes

- Replace SQLite storage of exact-match canonical responses and hit counters with namespaced, versioned Redis storage and native entry expiration.
- Keep cache enablement and TTL configuration in SQLite and continue reading those settings live per request.
- Add Redis connection configuration for the server, defaulting to a local Redis endpoint while allowing URL and namespace overrides without exposing credentials.
- Treat Redis initialization and runtime failures as cache misses or skipped writes so upstream model requests continue without a SQLite cache fallback.
- Preserve deterministic eligibility, semantic keys, request controls, CIF encoding, cross-dialect and cross-stream replay, response headers, and route scope.
- Preserve authenticated cache settings, statistics, and clear operations while reporting Redis backend availability and limiting administration to OmniLLM's namespace.
- Remove the obsolete SQLite response-cache table through a forward-only migration without migrating its disposable contents.
- Leave provider-owned prompt caching, provider model-list caching, metering, and all other durable state unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `caching`: Move exact-response payload storage and lifecycle to optional fail-open Redis, define native TTL behavior, namespace isolation, and degraded administration.
- `persistence`: Remove exact-response payloads from SQLite while retaining cache settings and all durable application records there.
- `cli-ops-config`: Add Redis startup configuration and expose non-fatal cache-backend availability to operators.
- `admin-api`: Extend response-cache administration with backend availability while preserving existing fields and operations during degraded storage.

## Impact

- Code: response-cache storage abstraction and Redis client, server startup/shutdown, chat and Messages cache calls, admin handlers, CLI configuration/status, and SQLite migration code.
- Dependencies: add the official Go Redis client and an in-process Redis-compatible test dependency; production deployments may supply Redis or Valkey externally.
- APIs: existing response-cache admin fields and commands remain compatible; backend and availability fields are additive. Liveness endpoints remain independent of Redis.
- Operations: upgrades begin with a cold response cache, desktop and server startup remain functional without Redis, and Redis URLs may include authentication, database, or TLS configuration.
