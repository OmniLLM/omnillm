## 1. Specification and Regression Tests

- [x] 1.1 Run `bun run spec:validate` and resolve all strict validation failures before implementation.
- [x] 1.2 Add focused Responses ingestion tests for sanitized Droid `ApplyPatch` custom calls and outputs, exact raw-input normalization, ordering, association, mixed function/custom history, empty values, missing fields, content-list output, invalid shapes, and custom tool definitions.
- [x] 1.3 Add a Responses-specific custom-tool compatibility fixture and canonical assertions without adding it to cross-dialect semantic equivalence scenarios.
- [x] 1.4 Add `/v1/responses` route coverage proving valid Droid-style history reaches provider dispatch and malformed custom items retain structured HTTP 400 behavior.

## 2. Responses Ingestion

- [x] 2.1 Extend Responses input decoding to preserve `input` and `output` field presence and supported wire value types while retaining existing function-item inference.
- [x] 2.2 Normalize `custom_tool_call` into an existing CIF tool call with the same `call_id` and name and one raw string argument named `input`.
- [x] 2.3 Normalize `custom_tool_call_output` into an existing CIF tool result, preserving string output or compactly encoding a validated ordered content list.
- [x] 2.4 Normalize named custom tool definitions into existing CIF tool definitions with one required string `input` property and no additional properties.
- [x] 2.5 Confirm existing function-call ingestion and provider serialization remain unchanged.

## 3. Focused Verification

- [x] 3.1 Run `go test ./internal/ingestion -run 'Responses|Compatibility'`.
- [x] 3.2 Run `go test ./internal/server -run 'CompatibilityAgentic|Responses'`.
- [x] 3.3 Replay a sanitized factory-cli 0.195.0 custom-tool continuation and verify provider dispatch, raw input preservation, ordering, and call/result association without enabling payload trace logs.
- [x] 3.4 Run bounded live multi-turn tool-call smoke tests through `jzhu/gpt-5.6-sol` for Claude Code, Codex CLI, and Droid, requiring at least two sequential tool calls and a terminal continuation; record any concrete client/configuration skip reason. Claude Code and Droid completed two calls plus terminal responses; Codex reached repeated Responses tool turns without gateway errors but could not complete because Codex 0.147 rejected the provider-emitted `functions__exec` name as an unsupported local tool call.
- [x] 3.5 Update project agent guidance so future tool-call compatibility changes require deterministic coverage and bounded live smoke tests for Claude Code, Codex CLI, and Droid when available.

## 4. Repository Verification and Completion

- [x] 4.1 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 4.2 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 4.3 Review the completed diff for scope, logging safety, and conformance to the approved focused-normalization design.
- [x] 4.4 Archive with `bunx openspec archive 2026-08-13-normalize-responses-custom-tool-history --yes` only after every task and verification check passes.
