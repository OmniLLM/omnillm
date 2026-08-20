## 1. Contract and Regression Tests

- [x] 1.1 Add response-cache store tests for namespace lookup-hit and lookup-miss semantics, bypasses, backend failures, malformed entries, statistics-window initialization, and clear reset.
- [x] 1.2 Add Redis statistics tests for live canonical payload bytes, compatibility `total_hits`, concurrent counter increments, namespace isolation, and neutral unavailable values.
- [x] 1.3 Add admin endpoint tests for additive JSON fields, hit-rate derivation, nullable no-observation values, degraded HTTP 200 behavior, authentication, and namespace clear.
- [x] 1.4 Add CLI tests for `cache stats` command registration, table rendering, raw JSON output, unavailable values, degraded Redis, and compatibility with older server responses.

## 2. Redis Cache Statistics

- [x] 2.1 Extend the response-cache statistics contract with payload bytes, lookup hits, lookup misses, optional hit rate, and optional statistics-window start.
- [x] 2.2 Add namespace metadata keys and atomic lookup outcome accounting while excluding disabled/ineligible bypasses and backend failures.
- [x] 2.3 Extend the batched live-entry scan to aggregate canonical payload bytes without counting namespace metadata as entries.
- [x] 2.4 Reset entries and namespace statistics together during a successful clear while preserving unrelated Redis keys.

## 3. API and CLI

- [x] 3.1 Extend the authenticated response-cache settings response with compatible statistics fields and neutral degraded values without exposing Redis configuration secrets.
- [x] 3.2 Implement and register `omnillm cache stats` using the existing admin client, table conventions, global flags, and raw JSON path.
- [x] 3.3 Format bytes, ratio, timestamp, and unavailable values consistently in human-readable output while clearly labeling local exact-response lookup statistics.

## 4. Documentation and Verification

- [x] 4.1 Document `omnillm cache stats`, its output semantics, and its distinction from provider prompt-cache usage in `README.md` and `README.zh-CN.md`.
- [x] 4.2 Run `bun run spec:validate` and `bun run spec:check`.
- [x] 4.3 Run `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 4.4 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 4.5 Check every task only after its implementation and verification passes, then archive with `bunx openspec archive add-cache-stats-command --yes`.
