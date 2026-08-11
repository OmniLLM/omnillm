## 1. Contract and Coverage Manifest

- [x] 1.1 Run `bun run spec:validate` and confirm the approved compatibility-testing capability is strict-valid before implementation begins.
- [x] 1.2 Define a test-only semantic fixture model covering plain text, one tool cycle, three sequential tool cycles, parallel/interleaved calls, mixed text and calls, empty arguments, large results, tool errors, thinking, completion, abrupt failure, and cancellation.
- [x] 1.3 Define a deterministic provider strategy manifest covering all supported provider types and distinct upstream paths, including Copilot Chat/Responses/Claude and OpenAI-compatible Chat/Responses rows.
- [x] 1.4 Add manifest completeness tests that fail when a supported provider strategy lacks a deterministic row or when an unsupported combination lacks an explicit not-applicable reason.

## 2. Dialect and Route Matrix

- [x] 2.1 Add shared builders that express each semantic fixture as OpenAI Chat Completions, Anthropic Messages, and OpenAI Responses input without duplicating expected canonical semantics.
- [x] 2.2 Expand ingestion tests so all three API shapes preserve equivalent messages, tools, call identifiers, arguments, results, ordering, and error relationships.
- [x] 2.3 Expand non-stream serialization tests so canonical fixture results produce valid envelopes and tool histories for all three API shapes.
- [x] 2.4 Expand streaming serialization tests for interleaved tool arguments, mixed content, usage, errors, and each dialect's terminal event without duplicate output.
- [x] 2.5 Expand route-level matrix tests for all three generation endpoints, root and `/v1` Chat equivalence, inbound shape annotation, model preservation, and result replay.
- [x] 2.6 Add deterministic three-cycle and parallel agentic route scenarios, including a large tool result and final answer without an unexpected tool call.

## 3. Provider Contract Matrix

- [x] 3.1 Add or extend fake-upstream harnesses so each provider strategy can assert endpoint, upstream model, request encoding, turns, tools, streaming strategy, and sanitized authentication shape.
- [x] 3.2 Add deterministic rows for GitHub Copilot Chat Completions, Responses, and Claude streaming strategies.
- [x] 3.3 Add deterministic rows for Antigravity, Alibaba, ModelScope, Azure OpenAI, Google, Kimi, Codex API key, and OpenAI OAuth execution strategies.
- [x] 3.4 Add deterministic OpenAI-compatible rows for Chat Completions and Responses configuration paths.
- [x] 3.5 Retain a thin end-to-end matrix that sends every inbound API shape through each representative provider strategy and validates downstream vocabulary plus upstream semantic history.

## 4. Long-Running, Cancellation, and Failure Behavior

- [x] 4.1 Add controllable response-header and stream-body fixtures that simulate long-running requests without production-duration sleeps.
- [x] 4.2 Verify long-lived streams remain active after timely headers and terminate only on valid completion, configured budget exhaustion, or caller cancellation.
- [x] 4.3 Verify cancellation prevents unintended retry or provider failover and never duplicates already emitted text or tool-call events.
- [x] 4.4 Verify transient retry boundaries and locally synthesized streaming strategies preserve a single coherent downstream response.

## 5. Isolated Live Matrix

- [x] 5.1 Implement an opt-in Bun/TypeScript live runner that exits successfully before build or network access unless `OMNILLM_RUN_LIVE_MATRIX=1` is set.
- [x] 5.2 Add a manifest schema and example configuration with provider/model capabilities and credential environment-variable or isolated token-bundle references, never secret values.
- [x] 5.3 Launch the real binary with a temporary HOME/config/database and automatically allocated loopback port; cleanly terminate it and remove temporary state after every run.
- [x] 5.4 Add bounded smoke execution for every runnable row: model availability, plain non-stream, plain stream, and one tool-call/result replay in every declared compatible API shape.
- [x] 5.5 Add extended execution for repeated and parallel tools, large results, long-lived streams, and cancellation where declared supported.
- [x] 5.6 Classify every planned row as pass, fail, skipped, or not applicable, fail supplied-but-invalid credentials, and produce sanitized console plus JSON reports.
- [x] 5.7 Add package commands and documentation for disabled, smoke, and extended live modes; retain or replace the existing fixed-port command with a compatibility path.

## 6. Verification and Completion

- [x] 6.1 Run focused ingestion, serialization, provider-dispatch, route, provider, server matrix, timeout, cancellation, and agentic tool-loop tests.
- [x] 6.2 Run the live command without opt-in and verify it performs no build, state access, launch, credential read, or network request.
- [x] 6.3 Run live smoke rows for all credentials available in the isolated test environment and record explicit skips for unavailable providers.
- [x] 6.4 Run extended live rows where credentials, provider capability, quota, and cost permit; ensure unavailable or unsupported rows are reported accurately.
- [x] 6.5 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 6.6 Run `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 6.7 Run `bun run spec:check`, review coverage and report sanitization, and confirm no production behavior or credentials changed.
- [x] 6.8 Archive with `bunx openspec archive add-model-compatibility-matrix-tests --yes` only after every deterministic task passes and live results have no unexplained failures.
