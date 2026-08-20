# Verification

Completed on 2026-08-20.

## Runtime lifecycle smoke tests

An isolated binary and temporary home directory verified that:

- `omnillm start --port 46137` created lifecycle state only after binding.
- `omnillm stop` delivered graceful termination, waited for exit, and removed
  lifecycle state.
- `omnillm restart --port 46139` terminated the original process, started a
  replacement on the requested port in the foreground, and allowed that
  replacement to be stopped cleanly.
- Root help advertises `start`, `stop`, and `restart` in the Server group.

## Automated verification

All required commands completed successfully:

- `bun run spec:check`
- `bun run lint:all`
- `bun run typecheck`
- `bun test` (361 passed, 22 intentionally skipped, 0 failed)
- `bun run build`
- `go vet ./...`
- `go build ./...`
- `go test -race ./...`

Focused race-enabled lifecycle, command, and server tests also passed. The
frontend build retained its pre-existing chunk-size advisory, and lint retained
its baseline-browser-mapping freshness advisory; neither is a failure.
