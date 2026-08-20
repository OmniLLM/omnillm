# Verification

Completed on 2026-08-20.

## Runtime and process tests

Isolated builds and temporary home directories verified:

- `start` returns after readiness while its detached PID remains alive and
  serves HTTP traffic.
- `restart` gracefully stops the first PID, returns after a distinct replacement
  PID is ready on the requested port, and that process remains alive.
- `stop` terminates the detached replacement and removes lifecycle state.
- `start --foreground` remains attached and records the attached PID.
- A real bind collision returns the child's concrete error and leaves no false
  lifecycle state.
- Subprocess regression tests cover readiness success, early child failure, and
  bounded timeout cleanup.
- The hidden child command stays absent from normal help, while `start` and
  `restart` both expose `--foreground` and default it to false.
- Windows cross-compilation (`GOOS=windows GOARCH=amd64 go build ./...`) passes.

The first implementation used an stdout readiness pipe. Runtime HTTP smoke
testing showed that Go created a parent-owned copy pipe for `io.Discard`; after
the launcher exited, later child output could receive SIGPIPE. The final
implementation uses an atomic operator-protected readiness file and direct
null-device descriptors, and the same traffic test passes after launcher exit.

## Required verification

All required commands completed successfully:

- `bun run spec:check`
- `bun run lint:all`
- `bun run typecheck`
- `bun test` (361 passed, 22 intentionally skipped, 0 failed)
- `bun run build`
- `go vet ./...`
- `go build ./...`
- `go test -race ./...`

Lint retained its dependency-freshness advisory and the frontend build retained
its existing chunk-size advisory; neither command failed.
