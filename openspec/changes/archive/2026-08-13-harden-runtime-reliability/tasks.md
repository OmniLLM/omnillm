## 1. Regression Coverage

- [x] 1.1 Add gateway handler tests for request bodies below, at, and above the limit across Chat Completions, Responses, Messages, and Messages token counting, including HTTP 413 envelopes and no dispatch.
- [x] 1.2 Add configuration-import tests for multipart requests and files below, at, and above the limit, verifying HTTP 413 preserves existing files.
- [x] 1.3 Add rate-limiter tests for prompt context cancellation, reservation rollback with later waiters, and unchanged active-request spacing.
- [x] 1.4 Add async-worker stress tests that race metering/token-stamp enqueue with stop and restart and complete without panic or deadlock under the race detector.
- [x] 1.5 Add Copilot concurrency coverage that refreshes model-shape metadata while request shape is selected under the race detector.
- [x] 1.6 Add persistence fault tests for late row-iteration errors and failed virtual-model, upstream, provider-instance, and model-state snapshot loads.

## 2. Request and Upload Bounds

- [x] 2.1 Add a shared bounded gateway-body reader with explicit oversized-request classification and stable dialect error responses.
- [x] 2.2 Apply the shared limit to Chat Completions, Responses, Messages, and Messages token-counting handlers before parsing.
- [x] 2.3 Apply request and uploaded-file limits to configuration imports before multipart parsing and file persistence.

## 3. Cancellation and Concurrency Safety

- [x] 3.1 Make rate-limit waits context-aware and retract canceled future reservations without delaying active waiters.
- [x] 3.2 Make async database enqueue and worker stop/restart mutually safe while preserving non-blocking overload behavior and draining accepted writes.
- [x] 3.3 Publish and read immutable Copilot model-shape snapshots with synchronization while preserving cache-miss heuristics.

## 4. Persistence Failure Integrity

- [x] 4.1 Refactor virtual-model cache loading to return failures and publish only complete virtual-model and upstream snapshots.
- [x] 4.2 Refactor provider-instance cache loading to return failures and publish only complete snapshots.
- [x] 4.3 Refactor model-state cache loading to return failures and publish only complete snapshots.
- [x] 4.4 Return row-iteration errors from virtual-model and virtual-model-upstream collection stores, and fix any equivalent confirmed collection readers covered by the same regression harness.

## 5. Dead-Code Analysis

- [x] 5.1 Add a focused Knip configuration for production, desktop, script, and test entrypoints plus TypeScript aliases and intentional standalone files.
- [x] 5.2 Re-run `bun run knip`, verify each remaining finding by repository search, and remove or narrowly mark only confirmed unused code or dependencies.

## 6. Verification

- [x] 6.1 Run `bun run spec:validate` before implementation and record human approval before checking implementation tasks.
- [x] 6.2 Run focused gateway, admin import, rate-limit, async-worker, Copilot, and persistence tests, including relevant `go test -race` packages.
- [x] 6.3 Run `bun run knip`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 6.4 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 6.5 Run `bun run spec:check`, confirm every task is checked, and archive `harden-runtime-reliability` only after all verification passes.
