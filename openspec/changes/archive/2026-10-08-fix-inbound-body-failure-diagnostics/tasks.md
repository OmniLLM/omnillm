## 1. Specification and approval

- [x] 1.1 Trace the reported failures through body reading, handler responses, HTTP logging, and log-stream formatting; review applicable current-state specs.
- [x] 1.2 Prepare proposal, gateway and CLI delta specs, design, and ordered verification tasks.
- [x] 1.3 Run `bun run spec:validate` with strict validation successfully (15 items passed, zero failed).
- [x] 1.4 Obtain human approval of proposal, specs, design, and tasks before code or test edits (user: "can you fix it").

## 2. Regression coverage before implementation

- [x] 2.1 Add bounded-reader fixtures for partial bytes with wrapped EOF, cancellation, unrelated read errors, body boundaries, and byte-count retention.
- [x] 2.2 Add failure-path coverage across Chat Completions, Responses, Messages, token counting, and System One for severity, status/envelope, metadata, and absence of dispatch.
- [x] 2.3 Add privacy sentinel and UTF-8 metadata-boundary checks, including query exclusion from access logs.
- [x] 2.4 Add a real loopback HTTP truncated-upload reproduction and HTTP field-preservation checks through middleware, broadcast formatting, CLI rendering, and existing console parsing.

## 3. Implementation

- [x] 3.1 Retain byte counts in failed bounded reads while preserving error chains and preventing partial-body parsing.
- [x] 3.2 Apply shared safe diagnostic classification to all bounded-reader callers without changing existing response contracts.
- [x] 3.3 Emit query-free access paths and restore HTTP method, path, and status in server-formatted log streams.
- [x] 3.4 Run focused regressions and confirm incomplete uploads never reach providers or trigger gateway retries.

## 4. Verification and archive

- [x] 4.1 Run existing deterministic five-turn client-shape coverage for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI with a terminal continuation.
- [x] 4.2 Run bounded five-tool-call live checks for each available configured client on isolated fresh-port gateways; record sanitized counts and terminal evidence or concrete prerequisite skip reasons.
- [x] 4.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 4.4 Run `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 4.5 Run `bun run spec:check` and record verification evidence and any remaining transport uncertainty.
- [x] 4.6 Confirm all implementation and verification tasks are complete and the approved change is ready for the OpenSpec archive. Run archive and post-archive `bun run spec:check` as lifecycle operations recorded in verification.md.
