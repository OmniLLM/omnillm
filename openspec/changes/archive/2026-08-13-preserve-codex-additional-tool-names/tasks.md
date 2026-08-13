## 1. Specification and CIF Tests

- [x] 1.1 Run strict OpenSpec validation and obtain human approval for the expanded CIF, provider, and public Responses design.
- [x] 1.2 Add CIF tests for function zero-value compatibility, custom definitions/formats, raw and empty input, original outputs, and JSON round trips.
- [x] 1.3 Add ingestion tests for declared name preservation, duplicate handling, native kind/format, two custom calls, exact raw inputs, outputs, ordering, and mixed function/custom history.

## 2. Native CIF and Ingestion

- [x] 2.1 Add additive custom-tool kind, format, raw input, namespace, original output, and raw-input delta fields to CIF serialization.
- [x] 2.2 Preserve native custom definition/call/output fields during Responses and `additional_tools` ingestion while retaining current fallback fields.
- [x] 2.3 Ensure function argument normalization and generic stream collection do not parse or discard custom input metadata.

## 3. Copilot and Public Responses

- [x] 3.1 Emit native custom definitions and history from the Copilot Responses request builder.
- [x] 3.2 Parse native non-streaming Copilot custom calls and tool-use stop reasons.
- [x] 3.3 Parse custom output-item and raw-input delta/done SSE events without duplication or cross-association.
- [x] 3.4 Emit native custom calls and custom-input streaming events from public Responses serialization.
- [x] 3.5 Add focused Copilot payload/parser/SSE and public serializer/SSE tests while preserving function-call tests.

## 4. Acceptance and Completion

- [x] 4.1 Run focused CIF, ingestion, Copilot, serialization, server, and compatibility tests.
- [x] 4.2 Run a bounded live Codex two-call `exec` smoke through `jzhu/gpt-5.6-sol` to terminal completion.
- [x] 4.3 Re-run bounded Claude Code and Droid two-call smoke tests to terminal completion.
- [x] 4.4 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, `bun run build`, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 4.5 Review the final diff, sync all validated deltas, and archive the completed change.
