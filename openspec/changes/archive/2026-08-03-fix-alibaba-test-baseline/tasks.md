## 1. Baseline Diagnosis and Tests

- [x] 1.1 Run `bun run spec:validate` and confirm this follow-up is approved before implementation.
- [x] 1.2 Reproduce and isolate the Alibaba `enable_thinking` failures in provider and server integration tests without external network dependencies.
- [x] 1.3 Reproduce and isolate the Alibaba live-model metadata naming failure with a deterministic model-list fixture.
- [x] 1.4 Determine from supported upstream behavior whether each failure requires a production correction or a stale-test correction, and update the approved artifacts if the normative contract changes.

## 2. Alibaba Baseline Corrections

- [x] 2.1 Align Qwen 3.6 Plus request construction and focused assertions for plain chat, tool calls, tool loops, and locally re-streamed Anthropic responses.
- [x] 2.2 Align live Alibaba model metadata enrichment and focused assertions while preserving stable model identifiers.
- [x] 2.3 Run Alibaba provider tests and Alibaba server integration tests with the race detector.

## 3. Repository Verification

- [x] 3.1 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 3.2 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 3.3 Confirm every checklist item and verification passes before archiving.
