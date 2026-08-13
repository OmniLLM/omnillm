## Context

See `proposal.md` for motivation. The affected paths span public HTTP handling, asynchronous persistence, in-memory routing caches, provider metadata, and repository analysis. The current code favors bounded non-blocking work queues and lazily loaded immutable cache snapshots, but several lifecycle edges bypass those intended contracts.

## Goals / Non-Goals

**Goals:**

- Preserve successful request and routing behavior while making fault and cancellation paths explicit.
- Use finite memory bounds at every client-controlled body ingestion point in scope.
- Guarantee that concurrent readers observe complete provider and persistence cache snapshots.
- Make the repository's existing Knip command useful as a repeatable verification gate.

**Non-Goals:**

- Change model routing, provider priority, rate-limit spacing, database schema, or queue capacities.
- Add configurable body-limit settings in this change; named constants provide stable initial contracts.
- Redesign all provider caches or all persistence stores beyond the identified failure modes.
- Remove code based solely on the current unconfigured Knip output.

## Decisions

### Use request-scoped bounded readers and explicit 413 mapping

Generation handlers will share a bounded body-reading helper that reads at most the limit plus one byte, distinguishes oversize from ordinary read failure, and emits the existing dialect error envelope with HTTP 413. Config import will apply `http.MaxBytesReader` before multipart parsing and retain a separate uploaded-file bound.

This is preferred over relying on JSON decoder behavior because decoding does not establish an allocation bound, and over server-wide limits because admin imports and gateway JSON bodies require distinct limits and responses.

### Make rate-limit reservations cancellable and reversible

`CheckAndWait` will accept `context.Context`, wait with a timer/select, and remove a canceled reservation while holding the limiter lock. Reservation accounting will be represented so cancellation can only retract the caller's own future slot and does not move the schedule behind already-issued reservations.

This is preferred over only replacing `time.Sleep` with a timer because that would release the goroutine but leave abandoned capacity delaying later callers.

### Synchronize queue send and close without blocking shutdown under the lifecycle mutex

Async worker state will prevent queue closure from racing with a selected send. Shutdown will detach/mark queues unavailable under the mutex, perform closure after no enqueuer can target them, and wait outside the lifecycle mutex. Enqueue remains non-blocking and may drop work once shutdown begins.

Holding the existing mutex across the current non-blocking send is acceptable, but waiting for worker drain while holding it is not; the design separates state transition from drain waiting.

### Publish immutable Copilot shape snapshots

Successful model discovery will build a fresh map and publish it atomically (or under a dedicated read/write lock); readers will never access a map being replaced or mutated. Existing heuristic fallback remains authoritative before a snapshot exists or on cache miss.

A provider-wide existing mutex is not assumed because token refresh and shape lookup have different contention and lifecycle concerns.

### Fail closed on incomplete database snapshots

Cache loaders will build local maps/slices, check every query, scan, and `rows.Err`, and publish only after all related reads succeed. On failure, the loaded flag remains false and any prior valid snapshot is preserved until explicit invalidation semantics require a retry. Store collection methods in scope will return `rows.Err()`.

This is preferred over logging and skipping malformed rows because routing cannot distinguish an intentionally absent row from a silently discarded one.

### Configure, then trust, dead-code analysis

A Knip configuration will enumerate Vite/desktop entrypoints, scripts, tests, TypeScript project files, aliases/workspaces as needed, and ignored generated or intentionally standalone files. Only findings that remain after configuration and are confirmed by repository search will be removed or marked intentionally retained.

This avoids treating the present output—where nearly every application file appears unused—as evidence for deletion.

## Risks / Trade-offs

- **[Body limits reject previously accepted very large payloads]** → Use conservative documented constants and boundary tests for below, equal, and above-limit payloads.
- **[Reservation rollback is subtle with concurrent waiters]** → Add deterministic limiter tests with multiple reservations and cancellation orders; run under the race detector.
- **[Worker lifecycle synchronization can deadlock during restart]** → Never wait on worker completion while holding a mutex needed by enqueue or worker progress; stress restart/enqueue under `-race`.
- **[Cache errors are not currently returned by public lookup APIs]** → Keep lookups' existing signatures, leave caches unloaded on failure, and make subsequent access retry rather than publishing absence.
- **[Knip may flag intentional public exports]** → Confirm each remaining finding against build/test consumers and encode intentional entrypoints or ignores narrowly rather than broad suppression.

## Migration Plan

1. Land body limits and cancellation behavior with regression tests; no persisted data migration is required.
2. Land concurrency and cache publication changes with race and injected-error coverage.
3. Add Knip configuration, resolve only verified findings, and require a clean `bun run knip` result in final verification.
4. Rollback is code-only: reverting restores previous behavior without schema or data changes.
