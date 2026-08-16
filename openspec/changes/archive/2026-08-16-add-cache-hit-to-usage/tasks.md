## 1. Specification and Approval

- [x] 1.1 Run `bun run spec:validate` and obtain human approval of the proposal, four delta specs, design, and tasks before implementation.

## 2. Backend Metering Tests

- [x] 2.1 Add raw metering record tests for cache-read positive, explicit zero, and null values deriving `hit`, `miss`, and `unknown` respectively.
- [x] 2.2 Add summary and model/provider/client breakdown tests for cache read/write token totals and hit/miss/unknown counts under existing filters.
- [x] 2.3 Add prompt-cache-status filter tests proving unknown rows are not counted as misses.
- [x] 2.4 Add API serialization tests for nullable cache detail and additive backward-compatible response fields.

## 3. Backend Metering Implementation

- [x] 3.1 Add the shared Go status type and derivation helper without adding a persisted status column.
- [x] 3.2 Extend raw metering records and query filters with derived prompt-cache status.
- [x] 3.3 Extend summary and grouped SQL queries/types with cache token totals and hit/miss/unknown counts while preserving existing totals.
- [x] 3.4 Update authenticated admin metering handlers and request parsing for the new optional status filter.

## 4. CLI Usage Output

- [x] 4.1 Add CLI tests for cache summary, grouped cache columns, unknown handling, and unchanged JSON pass-through.
- [x] 4.2 Extend human-readable usage summary and breakdown tables with provider prompt-cache status counts and cache read/write token totals.

## 5. Browser Usage Page

- [x] 5.1 Extend frontend API types with nullable cache token fields, prompt-cache status, and aggregate cache counters.
- [x] 5.2 Add frontend tests for hit, miss, unknown, zero-vs-null handling, status filtering, and narrow-layout rendering.
- [x] 5.3 Add provider prompt-cache summary cards or summary presentation with a known-status hit rate and visible unknown count.
- [x] 5.4 Add request-row and grouped-table cache status/read/write presentation while preserving existing identity, totals, sorting, pagination, and responsive behavior.
- [x] 5.5 Add localized provider prompt-cache labels, legend/help text, and hit/miss/unknown terminology.

## 6. Verification and Archive

- [x] 6.1 Run focused database, route, command, and frontend Metering tests.
- [x] 6.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, `bun run build`, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 6.3 Run code simplification and focused code review, fix verified findings, and rerun affected checks.
- [x] 6.4 Check every task and archive with `bunx openspec archive add-cache-hit-to-usage --yes` only after every verification passes.
- [x] 6.5 After archive, rerun `bun run spec:check` and the complete Bun and Go verification suite.
