## 1. Specification and Test Scaffolding

- [x] 1.1 Run `bun run spec:validate` and resolve every strict proposal, delta-spec, design, and task validation issue before implementation.
- [x] 1.2 Add hermetic Redis store contract tests for miss/hit, atomic concurrent hit accounting, overwrite reset, write-time native TTL and non-resurrection, malformed entries, namespace isolation, batched stats/clear, context deadlines, and idempotent close.
- [x] 1.3 Add response-cache service tests proving unavailable stores fail open, request contexts propagate, unsupported responses remain excluded, and backend recovery does not change keying or CIF encoding.
- [x] 1.4 Add route integration tests for Chat Completions and Anthropic Messages covering non-stream and stream miss/populate/hit, cross-dialect and cross-mode replay, bypass/off controls, cache headers, incomplete streams, and Redis read/write failure fallthrough.
- [x] 1.5 Add admin and CLI tests for additive backend availability, degraded statistics, clear errors, independent enabled/TTL updates, flag/environment/default precedence, doctor output, and credential redaction.
- [x] 1.6 Add migration tests proving the legacy SQLite response-cache table/index are removed while cache configuration and unrelated durable records survive.

## 2. Redis Store and Recovery

- [x] 2.1 Add the official Go Redis client and hermetic Redis test dependency, then create the context-aware response-cache `Store`, record/stat types, unavailable implementation, and concurrency-safe configuration seam.
- [x] 2.2 Implement validated versioned key construction and atomic hash save/get operations that preserve canonical response bytes, reset overwrite metadata, increment hits, retain expiration on reads, and apply positive native TTL on writes.
- [x] 2.3 Implement namespace-scoped cursor-based statistics and clear using bounded batches, pipelined metadata reads, `UNLINK` with compatible fallback, and local save/clear ordering without `KEYS` or `FLUSHDB`.
- [x] 2.4 Implement short command deadlines, disabled retry amplification, availability state, transition-only redacted logging, open/half-open circuit behavior, bounded background recovery probes, and race-safe idempotent shutdown.

## 3. Request and Administration Integration

- [x] 3.1 Refactor response-cache reads and writes to use the configured store with caller contexts while preserving eligibility, semantic keys, canonical encoding/decoding, bypass aliases, and best-effort behavior.
- [x] 3.2 Update Chat Completions and Anthropic Messages call sites to pass request contexts and preserve current stream accumulation, replay, thinking, tool-argument, and response-header behavior; keep Responses and embeddings untouched.
- [x] 3.3 Refactor authenticated response-cache handlers to use Redis statistics/clear, return durable settings with neutral degraded statistics, expose additive `backend` and `available` fields, and keep clear failure explicit.
- [x] 3.4 Update settings and doctor CLI output to display Redis backend availability while preserving existing table/JSON compatibility and never exposing a connection URL or credentials.

## 4. Startup, Lifecycle, and SQLite Retirement

- [x] 4.1 Add response-cache Redis URL and prefix startup options with flag, environment, and documented local-default precedence, URL parsing for Redis/TLS/auth/database settings, validation, and redacted diagnostics.
- [x] 4.2 Initialize/configure/ping Redis after SQLite with bounded contexts, continue startup in degraded mode on every Redis failure, and refactor server cleanup so workers, Redis, and SQLite close on listener errors and signal shutdown.
- [x] 4.3 Remove response-cache table/index creation from fresh SQLite schema, preserve historical migration 15 unchanged, append the forward drop migration, and remove the SQLite response-cache store and tests after all callers move.
- [x] 4.4 Verify server, container, and desktop-compatible `/healthz` startup without Redis and confirm Redis state does not alter `/`, `/health`, or `/healthz` liveness.

## 5. Documentation and Compatibility Verification

- [x] 5.1 Update README and CLI help with local Redis/Valkey setup, URL/prefix precedence, auth/TLS examples, container service-host guidance, native TTL behavior, cold upgrade, fail-open/no-fallback semantics, and the distinction from provider prompt caching.
- [x] 5.2 Run deterministic multi-turn tool-use regression coverage for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider shapes across affected Chat Completions and Messages paths.
- [x] 5.3 Run bounded live smoke tests with at least five sequential native tool calls for each available client/model configuration, recording sanitized call/result counts and terminal evidence or a concrete skip reason.
- [x] 5.4 Run focused tests plus `gofmt`, `go mod tidy`, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 5.5 Run `bun run lint:all`, `bun run typecheck`, `bun test`, `bun run build`, and `bun run spec:check`; build relevant Docker/desktop artifacts and resolve every failure.
- [x] 5.6 With a disposable real Redis, verify miss/write/hit, stream replay, bypass refresh, off, statistics, TTL expiration, namespace clear, outage fast-fail, recovery, credential redaction, and startup without Redis.

## 6. Archive

- [x] 6.1 Confirm every task is checked and all verification evidence passes, archive with `bunx openspec archive 2026-08-16-move-response-cache-to-redis --yes`, and rerun `bun run spec:check`.
