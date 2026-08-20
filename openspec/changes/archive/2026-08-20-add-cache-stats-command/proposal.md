## Why

Operators can inspect exact-response cache settings and a live-entry hit aggregate, but they cannot see cache storage usage, lookup misses, or a meaningful hit rate from the CLI. A dedicated statistics command will make Redis-backed response-cache effectiveness and footprint observable without conflating it with upstream provider prompt caching already reported by `omnillm usage`.

## What Changes

- Add `omnillm cache stats` with human-readable table output and raw server JSON through the existing global `--output json` mode.
- Extend authenticated response-cache statistics with live payload bytes plus namespace-scoped lookup hit and miss counters, hit rate, and the beginning of the reported statistics window.
- Preserve the existing response-cache settings command and compatibility fields, including the legacy live-entry `total_hits` aggregate.
- Return neutral statistics with explicit degraded availability when Redis statistics cannot be read.
- Document the distinction between local exact-response lookup statistics and provider prompt-cache usage.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `caching`: Define exact-response cache payload usage and namespace-scoped lookup hit/miss accounting, including degraded behavior.
- `admin-api`: Add compatible response-cache statistic fields to the authenticated settings response.
- `cli-ops-config`: Add the `cache stats` administration command and its table/JSON output contract.

## Impact

- CLI command registration, rendering, and tests.
- Redis response-cache storage/statistics implementation and tests; no response payloads or counters move into SQLite.
- Authenticated response-cache admin API and integration tests.
- Operator documentation in both README variants.
- No breaking API or command changes and no new dependency.
