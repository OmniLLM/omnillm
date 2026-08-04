## 1. Specification and Test Contract

- [x] 1.1 Add zerolog capture helpers and synthetic secret sentinels that support record-wide absence assertions while preserving metadata assertions.
- [x] 1.2 Add regression cases for nested non-streaming arguments, streamed argument deltas, plain and structured results, raw and canonical inbound forms, and errored Agent warning events.

## 2. Metadata-Only Diagnostics

- [x] 2.1 Remove `tool_arguments`, `tool_result`, and `raw_inbound_payload` content from all tool-loop and Agent failure log events while retaining correlation, tool, index, model/provider, stream, and error metadata.
- [x] 2.2 Add argument and result byte-length metadata where payload data is already available without additional sensitive copies.
- [x] 2.3 Remove raw-argument, partial-JSON, preview extraction, truncation, or parsing state that exists solely for payload logging, while preserving the streaming tracker metadata and Agent guardrail state and without changing request translation or response behavior.
- [x] 2.4 Update existing tool-loop logging tests that currently require verbatim payload previews to require metadata-only events instead.

## 3. Verification

- [x] 3.1 Run `gofmt` on changed Go files and run focused route/server tests, including the new privacy regressions.
- [x] 3.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 3.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 3.4 Review the final diff to confirm no public API, routing, CIF, or provider behavior changed and no payload-valued logging path remains in scope.
