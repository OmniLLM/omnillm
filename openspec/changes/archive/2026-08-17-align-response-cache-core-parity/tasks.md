## 1. Specification and approval gate

- [x] 1.1 Run `bun run spec:validate` and `openspec status --change align-response-cache-core-parity --json`, fix every strict validation error, and confirm proposal, specs, design, and tasks are complete.
- [x] 1.2 Present the complete validated OpenSpec artifacts for explicit human approval and record that approval before modifying tests, runtime code, documentation, or generated current-state specs.

## 2. Failing cache contract tests

- [x] 2.1 Update response-cache eligibility and TTL tests to require omitted, zero, and nonzero sampling controls to be eligible and the invalid/absent fallback TTL to be 60 seconds while positive configured TTLs remain authoritative.
- [x] 2.2 Add field-by-field semantic-key tests for Responses state, generation controls, stream/dialect exclusions, nested and top-level provider prompt-cache neutrality, and fail-closed key serialization.
- [x] 2.3 Add cache codec tests proving lossless native custom-tool kind, raw input presence including empty input, namespace, identifiers, arguments, order, and backward-compatible legacy function-call decoding.
- [x] 2.4 Add stream accumulator and synthesis tests for custom-input deltas, interleaved function/custom calls, explicit empty input, block/index reuse, one terminal event, and exclusion of errored or incomplete streams.

## 3. Failing route and metering tests

- [x] 3.1 Extend response-cache integration tests with Chat, Messages, and Responses non-stream population/replay, omitted and nonzero sampling, hit/miss headers, bypass/off behavior, and exact upstream call counts.
- [x] 3.2 Add true upstream-stream population and synthetic replay tests for all three dialects, including stream-to-non-stream and non-stream-to-stream reuse, one terminal event, no duplicate output, and no write on cancellation/error/incomplete closure.
- [x] 3.3 Add Responses function/custom tool cache tests covering raw multiline input, namespace, explicitly empty input, correct native event vocabulary, structured function-result continuation, and distinct `previous_response_id` chains.
- [x] 3.4 Add cache conversion-failure and unavailable/corrupt Redis tests proving fail-open provider execution and no false served-hit accounting.
- [x] 3.5 Add asynchronous metering regressions proving misses retain provider cache detail while served hits use response-cache attribution, preserve aggregate volume, and derive provider prompt-cache status `unknown`.

## 4. Core cache identity and payload implementation

- [x] 4.1 Change the fallback TTL to 60 seconds and remove sampling-based checks from request eligibility without changing opt-in enablement or positive persisted TTL behavior.
- [x] 4.2 Implement a reviewed semantic key projection containing all supported generation semantics, including `PreviousResponseID` and thinking controls, while excluding stream, dialect, transport/routing metadata, and provider prompt-cache controls.
- [x] 4.3 Make semantic-key encoding return no usable key on serialization failure and update every cache caller to proceed upstream without a read or write.
- [x] 4.4 Extend the cached canonical tool projection with backward-compatible optional kind, raw input presence/value, and namespace fields.

## 5. Streaming and Responses implementation

- [x] 5.1 Extend canonical stream accumulation to preserve native custom-tool metadata and custom raw-input deltas independently from function arguments.
- [x] 5.2 Extend synthetic stream generation to announce and emit function and custom tool calls using their correct delta vocabularies, order, and single terminal boundary.
- [x] 5.3 Integrate `/v1/responses` cache lookup and non-stream replay through the existing Responses serializer, committing hit headers only after successful conversion and falling through safely otherwise.
- [x] 5.4 Integrate Responses stream replay through existing CIF-to-Responses SSE serialization and add complete-stream population after normalized successful terminal events.
- [x] 5.5 Align Chat and Messages with safe key failure, conversion-committed hit behavior, consistent cache logging, and shared served-hit metering without changing their routing/control order.

## 6. Compatibility coverage

- [x] 6.1 Extend deterministic Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider fixtures to exercise cache population/replay with five sequential native tool calls, five associated results, terminal continuation, and no sixth call.
- [x] 6.2 Verify native Responses custom tools and structured function results retain kind, IDs, names, raw input, namespace, values, ordering, and correct stream vocabulary through cached replay.
- [x] 6.3 Extend the live model matrix manifest/schema/tests with an exact-response-cache capability and scenario that is explicitly separate from provider prompt-cache proof.
- [x] 6.4 Run bounded live client smokes for every locally available configured client against isolated fresh-port gateway and Redis state; record sanitized versions, five calls/results, terminal marker, hit/miss evidence, exit status, and concrete skip reasons for unavailable prerequisites.

## 7. Documentation and focused verification

- [x] 7.1 Update English and Chinese README cache documentation for opt-in behavior, 60-second fallback, three generation dialects, sampling-independent replay, canonical rather than wire/timing equality, provider-cache separation, and OmniLLM-specific Redis format.
- [x] 7.2 Run focused Go tests for responsecache, routes/server, ingestion, serialization, database, commands, and provider compatibility, including the maintained compatibility route/tool-loop patterns.
- [x] 7.3 Run a disposable real-Redis smoke with race detection and verify save/hit, TTL expiry, overwrite, namespace isolation, clear behavior, outage recovery, and unrelated-key preservation.
- [x] 7.4 Run live-matrix safe-no-op evidence without opt-in and credential-gated smoke/extended runs when configured, recording sanitized results and concrete skips.

## 8. Full verification and review

- [x] 8.1 Run `gofmt` on changed Go files, `go vet ./...`, `go build ./...`, and `go test -race ./...` successfully.
- [x] 8.2 Run `bun run lint:all`, `bun run typecheck`, `bun test`, `bun run build`, `bun run spec:check`, and `git diff --check` successfully.
- [x] 8.3 Run the project code simplifier on the implementation diff, apply only behavior-preserving improvements, and rerun affected tests.
- [x] 8.4 Run focused code review for correctness, silent failures, cache identity, custom-tool fidelity, streaming uniqueness, metering separation, security/privacy, and test coverage; resolve every verified finding and rerun affected checks.
- [x] 8.5 Record all commands, results, sanitized live evidence, skipped prerequisites, known trade-offs, and final clean status in `verification.md`.

## 9. Archive

- [x] 9.1 Confirm every implementation task is checked and no verification failure or unresolved finding remains.
- [x] 9.2 Run `bunx openspec archive align-response-cache-core-parity --yes`, then rerun `bun run spec:check` and verify the archived delta produced the intended current-state specifications.
