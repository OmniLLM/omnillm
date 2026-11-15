## 1. Specification
- [x] 1.1 Run strict OpenSpec validation and obtain human approval of proposal, delta specs, design, and tasks before implementation.

## 2. Regression coverage
- [x] 2.1 Add deterministic refresh tests for expiry, legacy records, missing credentials, invalid grants, HTTP and malformed-response failures, cancellation, and secret-safe errors.
- [x] 2.2 Add persistence and concurrency tests for omitted/rotated refresh tokens, metadata, restart, failed writes, simultaneous requests, concurrent OAuth completion, and lifecycle lease safety.
- [x] 2.3 Add streaming and non-streaming tests for 401 recovery, retry bounds, 403 exclusion, unchanged tools/history, and no replay after delivery.

## 3. Implementation
- [x] 3.1 Persist expiry on OAuth completion and add safe conditional refresh persistence.
- [x] 3.2 Implement coordinated, context-aware automatic and explicit refresh and propagate load errors.
- [x] 3.3 Add typed upstream HTTP errors and one pre-stream authentication retry.

## 4. Verification and delivery
- [x] 4.1 Run bun run spec:check, bun run lint:all, bun run typecheck, bun test, and bun run build.
- [x] 4.2 Run go vet ./..., go build ./..., and go test -race ./....
- [x] 4.3 Verify deterministic five-call tool loops for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI shapes; run bounded isolated live smokes when configured or record concrete skip reasons.
- [x] 4.4 Record sanitized verification results and archive only after all required checks and preceding tasks pass.
