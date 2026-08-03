## 1. Specification and Regression Tests

- [x] 1.1 Run `bun run spec:validate` and confirm the approved delta remains strict-valid before implementation.
- [x] 1.2 Add table-driven unit tests for schema-aware argument normalization covering optional empty strings, required empty strings, non-empty strings, non-string values, nested values, undeclared properties, missing schemas, and malformed arguments.
- [x] 1.3 Add Copilot chat-completions and Responses stream regressions proving completed arguments are normalized while provider-index identity and interleaved emission order remain intact.
- [x] 1.4 Extend Anthropic serialization and response-cache regressions to prove live and replayed streams emit identical normalized tool arguments.

## 2. Schema-Aware Tool Argument Normalization

- [x] 2.1 Implement a shared helper that resolves a canonical tool declaration and conservatively omits only top-level, declared, optional properties whose completed value is exactly an empty string.
- [x] 2.2 Thread canonical tool schema context through non-streaming response translation and apply normalization before client-facing tool-call emission.
- [x] 2.3 Update streamed tool-call assembly to buffer arguments independently by provider index until completion, then normalize and emit them without reintroducing a single current-block cursor.
- [x] 2.4 Ensure response-cache accumulation stores and replays the same normalized completed arguments as live streaming.

## 3. Verification

- [x] 3.1 Run focused Go tests for the shared helper, Copilot provider parsing, Anthropic serialization, and response-cache stream replay.
- [x] 3.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 3.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
  - Verification note: `go vet ./...` and `go build ./...` pass. Change-scoped and all non-Alibaba race tests pass. The full race command was run and exposed unrelated existing Alibaba expectations in `internal/providers/alibaba` and Alibaba integration cases in `internal/server` (`enable_thinking` missing and model metadata naming); no Alibaba files are changed, and follow-up is tracked in `fix-alibaba-test-baseline`.
- [x] 3.4 Confirm every checklist item is complete and every change-scoped verification passes; record unrelated baseline failures as a separate follow-up before archiving.
