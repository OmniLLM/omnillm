# Tasks

## 1. Specification

- [x] 1.1 Validate the proposal, delta spec, design, and tasks with strict
  OpenSpec validation.
- [x] 1.2 Obtain human approval for the validated change artifacts.

## 2. Tests

- [x] 2.1 Refactor lifecycle tests so Linux-only `/proc` assertions are
  platform-scoped while common lifecycle behavior runs on macOS.
- [x] 2.2 Add deterministic Darwin tests for native process identity,
  executable replacement stability, stale identity rejection, and managed
  process termination.

## 3. Implementation

- [x] 3.1 Extract the current Linux `/proc` identity implementation into a
  Linux build-tagged source file without changing its behavior.
- [x] 3.2 Implement CGO-free Darwin identity collection from
  `kern.proc.pid` and `PROC_PIDPATHINFO`, including strict payload validation
  and path normalization.
- [x] 3.3 Add an explicit unsupported-platform identity implementation so
  lifecycle registration fails safely where native support is absent.

## 4. Verification

- [x] 4.1 Run targeted lifecycle tests, including the Go race detector.
- [x] 4.2 Cross-compile the lifecycle package for Linux and Darwin and build
  the native macOS binary.
- [x] 4.3 Run a native macOS background start, status, and stop smoke test.
- [x] 4.4 Run `bun run spec:check`, `go vet ./...`, `go build ./...`, and
  `go test -race ./...`.
- [x] 4.5 Archive `support-darwin-managed-lifecycle` only after every task is
  complete and all verification passes.
