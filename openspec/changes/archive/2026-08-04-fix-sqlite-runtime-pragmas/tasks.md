## 1. Regression Coverage

- [x] 1.1 Add a database initialization test that queries and asserts effective WAL mode, a 5000 millisecond busy timeout, and enabled foreign keys.
- [x] 1.2 Add a bounded second-handle contention test that acquires a conflicting lock, releases it before the timeout, and verifies a metering insert waits and succeeds.

## 2. SQLite Connection Configuration

- [x] 2.1 Replace unsupported runtime SQLite DSN parameters with `modernc.org/sqlite` `_pragma` parameters while preserving the one-open/idle-connection pool policy.
- [x] 2.2 Find every maintenance script that opens `database.sqlite` directly and update it to apply the same supported WAL, busy-timeout, and foreign-key pragmas.
- [x] 2.3 Re-run the repository search for deprecated `_journal_mode`, `_busy_timeout`, and `_foreign_keys` DSN keys and confirm no OmniLLM database opener still uses them.

## 3. Provider Aggregate Integrity

- [x] 3.1 Add regression tests that reject child persistence without a parent, verify successful provider creation persists parent and children, and verify failed creation cleans up newly reserved data.
- [x] 3.2 Add parent-first provider-creation orchestration with failure cleanup while preserving existing-provider re-authentication behavior.
- [x] 3.3 Update direct provider and server test fixtures to seed provider-instance parents before child persistence.

## 4. Verification

- [x] 4.1 Run focused database tests, including the pragma and contention regressions, under the race detector.
- [x] 4.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 4.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 4.4 Confirm all tasks are complete and archive `fix-sqlite-runtime-pragmas` with `bunx openspec archive fix-sqlite-runtime-pragmas --yes` only after every verification passes.
