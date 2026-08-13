## 1. Specification and Deterministic Coverage

- [x] 1.1 Run strict OpenSpec validation and obtain human approval before implementation.
- [x] 1.2 Expand the shared sequential compatibility fixture from three to five exchanges and remove brittle positional lookup.
- [x] 1.3 Strengthen manifest and route assertions for five ordered calls/results, terminal continuation, and no sixth call.
- [x] 1.4 Add or extend native Responses custom-tool coverage so five Codex-style `exec` calls retain kind, raw input, IDs, outputs, and order.

## 2. Live Matrix and Policy

- [x] 2.1 Update repeated-tool live-matrix logic to five calls plus terminal continuation using a shared minimum constant.
- [x] 2.2 Extend live-matrix unit tests to assert five call/result histories for Chat, Messages, and Responses.
- [x] 2.3 Update `CLAUDE.md` to require five sequential calls for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom provider.
- [x] 2.4 Document the opt-in four-client smoke command/configuration, timeout, evidence, and skip/fail rules.

## 3. Live Client Acceptance

- [x] 3.1 Build and launch an isolated patched OmniLLM instance with a hard timeout and sanitized logs.
- [x] 3.2 Run Claude Code through five sequential `Bash` calls and terminal completion.
- [x] 3.3 Run Codex CLI through five sequential native custom `exec` calls and terminal completion.
- [x] 3.4 Run Droid through five sequential `Execute` calls and terminal completion.
- [x] 3.5 Run GitHub Copilot CLI through five sequential `shell` calls using its OpenAI Responses custom provider and terminal completion.

## 4. Completion

- [x] 4.1 Run focused Go and Bun compatibility tests.
- [x] 4.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, `bun run build`, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 4.3 Review the complete diff for regressions, scope, and credential/log safety.
- [x] 4.4 Sync the validated specification delta and archive the completed change.
