## 1. Specification and Regression Tests

- [x] 1.1 Run `bun run spec:validate` and confirm the human-approved change remains strict-valid before implementation.
- [x] 1.2 Add table-driven unit tests for exact sentinel matching, Claude Code product-token recognition, `Agent` and `subagent_type` scoping, direct `enum` and `const` authorization, and preserve-on-uncertainty behavior.
- [x] 1.3 Add deterministic Anthropic non-streaming and streaming route regressions proving the known value becomes `Plan` for Claude Code while near matches and non-Claude Code callers remain unchanged.
- [x] 1.4 Add response-cache regressions proving cache hits emit the same repaired call as live responses and unsafe legacy hits are bypassed rather than emitted inconsistently.
- [x] 1.5 Add metadata-only diagnostic tests proving repair records contain correlation metadata but no tool arguments, prompt content, or tool results.

## 2. Claude Code Compatibility Repair

- [x] 2.1 Add an explicit client compatibility policy derived from the Anthropic Messages request's parsed `claude-cli/<version>` product token.
- [x] 2.2 Implement a conservative completed-argument repair that matches only the exact Plan-mode sentinel and requires the declared `Agent.subagent_type` schema to allow `Plan` directly.
- [x] 2.3 Apply the policy to non-streaming and independently indexed streaming tool calls before Anthropic client emission without changing existing optional-empty normalization.
- [x] 2.4 Ensure live response caching and cache replay produce contract-equivalent repaired tool calls, bypassing unsafe legacy hits when necessary.
- [x] 2.5 Emit one metadata-only structured diagnostic for each applied repair.

## 3. Compatibility Verification

- [x] 3.1 Run focused Go tests for tool-argument normalization, Anthropic serialization/routes, response-cache replay, and tool-loop diagnostics.
- [x] 3.2 Run deterministic multi-turn tool-use coverage for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider API shapes, including at least five sequential calls and a terminal continuation where required by the repository policy.
  - Evidence: compatibility ingestion, serialization, cache-replay, Droid custom-tool, four-route agentic matrix, and five-cycle terminal-continuation suites passed.
- [x] 3.3 When the corresponding clients and local model configuration are available, run bounded live smoke tests for Claude Code with a non-Claude model plus every other supported coding-agent client; record sanitized call/result counts and terminal evidence, and record concrete reasons for any skipped client.
  - Claude Code 2.1.235 with `gpt-5.6-sol` through an isolated build completed five sequential native Read calls and a sixth terminal turn with `FIVE_CALLS_OK` (`num_turns: 6`, exit 0).
  - A separate Claude Code Plan-agent smoke emitted and locally launched `Agent.subagent_type: Plan`; the isolated catalog lacked the child-selected `claude-sonnet-5`, so the child exhausted upstream 502 retries, while the parent observed the result and terminated with `LIVE_OK` (exit 0). This proves the client lookup/launch boundary fixed by this change.
  - The direct live Messages probe against `gpt-5.6-sol` returned one valid `Agent` tool call with `subagent_type: Plan` and `stop_reason: tool_use`.
  - Codex CLI 0.148.0, Droid 0.199.0, and GitHub Copilot CLI 1.0.81-3 were present, but their live runs were skipped because the required isolated live-matrix manifest and client-specific credential/configuration declarations were absent. Their deterministic native API shapes passed in 3.2. The safe live-matrix command confirmed the credential-gated skip.
- [x] 3.4 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
  - Evidence: spec gate, ESLint, TypeScript, 361 Bun tests (22 intentional skips), and production Vite build passed.
- [x] 3.5 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
  - Evidence: all three commands passed across every Go package.
- [x] 3.6 Confirm every checklist item is complete and all required verification passes before archiving the change.
