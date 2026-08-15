## 1. Spec and Live Reproduction

- [x] 1.1 Run `bun run spec:validate` and obtain human approval of the proposal, provider and compatibility deltas, design, and tasks before implementation.
- [x] 1.2 Discover the exact Grok model IDs, capabilities, limits, and supported endpoints from the selected authenticated GitHub Copilot provider without exposing credentials.
- [x] 1.3 Add account-specific GitHub Copilot Grok rows to ignored `scripts/live-model-matrix.json`, provision into temporary state, and run smoke mode on a newly allocated port.
- [x] 1.4 Run extended mode in another fresh process and record sanitized failing scenario, selected upstream shape, status/code, call/result counts, and terminal evidence.

## 2. Deterministic Regression

- [x] 2.1 Add a fake Copilot catalog with the observed Grok metadata and reproduce the live failure in focused provider or routing tests.
- [x] 2.2 Add affected Chat Completions, Messages, and Responses stream/non-stream coverage using the five-cycle compatibility fixture, including terminal continuation and no sixth call.
- [x] 2.3 Add endpoint selection, attempt-count, cancellation, and concurrency/cache tests required by the reproduced defect while retaining existing Copilot strategy rows.
- [x] 2.4 Update the tracked live-matrix example, schema, and runner unit tests only where needed to represent safe Copilot Grok provisioning and reporting.

## 3. Evidence-Backed Implementation

- [x] 3.1 Correct the demonstrated Copilot catalog, endpoint-shape, pinned-route, payload, stream, or per-instance cache behavior without adding direct xAI support or unrelated compatibility changes.
- [x] 3.2 Run the focused Go compatibility packages and `bun test tests/scripts/live-model-matrix.test.ts`; fix any regression before proceeding.

## 4. Fresh-Port Verification and Full Gates

- [x] 4.1 Start a completely new isolated live-matrix gateway on another allocated port and pass Grok smoke mode.
- [x] 4.2 Pass Grok extended mode with five ordered tool calls/results, terminal continuation, parallel tools, large result, long stream, and cancellation for every applicable shape.
- [x] 4.3 Record native-client live smokes as skipped: although all four executables are installed, this repository has no reusable isolated client runner or safe temporary custom-provider configuration for Claude Code, Codex CLI, Droid, or GitHub Copilot CLI; deterministic five-call client-shape coverage passed.
- [x] 4.4 Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`.
- [x] 4.5 Run `bun test`, `bun run lint:all`, `bun run typecheck`, `bun run build`, and `bun run spec:check`.
- [x] 4.6 Check every completed task and archive with `bunx openspec archive fix-copilot-grok-compatibility --yes` only after all verification passes.
