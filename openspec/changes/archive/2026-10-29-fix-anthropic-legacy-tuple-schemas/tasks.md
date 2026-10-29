## 1. Approval and regression coverage

- [x] 1.1 Run `bun run spec:validate` and obtain human approval of the proposal, delta spec, design, and tasks before implementation.
- [x] 1.2 Add failing ingestion regressions for the sanitized RGB schema, trailing-item policies, nested schema positions, literal preservation, modern schemas, and input immutability.
- [x] 1.3 Add deterministic provider-boundary coverage proving the normalized tuple reaches upstream intact and tool-result continuation succeeds; retain five-turn fixtures for all four coding-agent clients.

## 2. Implementation

- [x] 2.1 Implement schema-aware, non-mutating legacy tuple normalization at the Anthropic tool ingestion boundary.
- [x] 2.2 Verify tool metadata and order remain intact and that mixed modern/legacy nodes are not destructively rewritten.

## 3. Verification and rollout

- [x] 3.1 Run focused tests, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 3.2 Run `bun run lint:all`, `bun run typecheck`, `bun test`, `bun run build`, and `bun run spec:check`.
- [x] 3.3 Run isolated bounded live checks for available coding clients with five sequential native calls/results and terminal continuation; record concrete skip reasons for unavailable clients.
- [x] 3.4 Repeat the sanitized original-tool live reproduction after implementation and record successful schema acceptance without executing MCP actions.
- [x] 3.5 Install and restart the service after approval and successful verification; verify the deployed binary and actual tool-bearing request behavior, retaining a recoverable prior binary.
- [x] 3.6 Record sanitized verification results, check completed tasks, and archive only after all required work and checks pass.
