# Tasks

- [x] 1.1 Strictly validate this proposal, delta specification, design, and task
  list and obtain human approval before implementation.
- [x] 1.2 Add failing tests for background start/restart, foreground mode,
  readiness success/failure/timeout, child cleanup, and command discovery.
- [x] 2.1 Refactor startup option resolution so foreground and child execution
  share identical flags, environment precedence, and validation.
- [x] 2.2 Add the hidden internal child entrypoint and bounded readiness/error
  handshake after listener binding and lifecycle registration.
- [x] 2.3 Add platform-specific detached child spawning, protected log routing,
  child reaping, timeout handling, and interruption cleanup.
- [x] 2.4 Make `start` and `restart` background by default and add the explicit
  `--foreground` compatibility mode.
- [x] 2.5 Update CLI help and operator documentation for background lifecycle
  behavior and foreground supervisor usage.
- [x] 3.1 Run isolated binary start, restart, stop, foreground, early-failure,
  and readiness-timeout smoke tests.
- [x] 3.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`,
  `bun test`, `bun run build`, `go vet ./...`, `go build ./...`, and
  `go test -race ./...`.
- [x] 3.3 Archive only after all tasks and verification pass.
