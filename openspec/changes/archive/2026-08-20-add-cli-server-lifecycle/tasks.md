# Tasks

- [x] 1.1 Validate the OpenSpec proposal, delta specification, design, and tasks
  strictly and obtain human approval before implementation.
- [x] 1.2 Add focused failing tests for lifecycle-state ownership, stale and
  malformed state, process identity validation, graceful-stop success and
  timeout, restart behavior, and root command discovery.
- [x] 2.1 Implement operator-local atomic lifecycle-state registration,
  validation, and ownership-safe cleanup with isolated process tests.
- [x] 2.2 Integrate lifecycle registration with successful foreground server
  startup and cleanup with every server exit path.
- [x] 2.3 Add `omnillm stop` with validated graceful signaling, bounded waiting,
  safe stale-state handling, and actionable output and errors.
- [x] 2.4 Add `omnillm restart` with the same startup flags and environment
  precedence as `start`, graceful stop when a managed server exists, and
  foreground startup afterward.
- [x] 2.5 Register and document both commands in the root Server command group.
- [x] 3.1 Run focused Go tests for commands and server lifecycle behavior.
- [x] 3.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`,
  `bun test`, `bun run build`, `go vet ./...`, `go build ./...`, and
  `go test -race ./...`.
- [x] 3.3 Archive the validated change only after every task is checked and all
  required verification passes.
