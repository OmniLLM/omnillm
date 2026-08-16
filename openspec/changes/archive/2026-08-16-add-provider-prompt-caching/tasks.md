## 1. Specification Approval and Baseline

- [x] 1.1 Run `bun run spec:validate` and obtain human approval of the proposal, seven delta specs, design, and tasks before implementation.
- [x] 1.2 Run the configured smoke live matrix in an isolated process on an automatically allocated port and record sanitized baseline outcomes and concrete skips in `verification.md`.
- [x] 1.3 Run the configured extended live matrix in a second isolated process and port, recording five-call, terminal, stream, cancellation, and provider-shape evidence.
- [x] 1.4 Inspect authenticated provider request/usage contracts without exposing credentials and convert any cache-related live failure into a fake-upstream regression before production edits.

## 2. Canonical Model and Ingestion Tests

- [x] 2.1 Add failing CIF tests for cache-control type/TTL, structured system blocks, cacheable content/tool placement, native cache policy, usage arithmetic, JSON round trips, and independent candidate copies.
- [x] 2.2 Extend shared compatibility fixtures with cache-bearing Claude Code, Codex, Droid, and Copilot CLI histories containing five sequential call/result cycles.
- [x] 2.3 Add failing Anthropic ingestion tests for top-level automatic control, every supported explicit placement, omitted/5m/1h TTL, system order, mid-conversation system messages, malformed values, unsupported placement, and the fifth-breakpoint error.
- [x] 2.4 Add failing Chat Completions and Responses ingestion tests for native prompt cache key/retention and unchanged legacy requests.

## 3. Canonical Model and Ingestion Implementation

- [x] 3.1 Add typed CIF cache control, TTL, request policy, structured system blocks, cache usage detail, and shared validation/arithmetic/rendering helpers.
- [x] 3.2 Migrate all production and test constructors from the authoritative flattened system prompt to ordered system blocks, preserving legacy provider text rendering where required.
- [x] 3.3 Implement Anthropic cache-control parsing and validation while preserving message/tool order and tool call/result associations.
- [x] 3.4 Implement Chat Completions and Responses native cache-key/retention ingestion and dialect-specific structured HTTP 400 handling.

## 4. Provider Contract Tests

- [x] 4.1 Add failing OpenAI-compatible configuration tests for `auto`, `disabled`, `openai_native`, and `anthropic_inline`, including official-host rejection and backward-compatible defaults.
- [x] 4.2 Add exact fake-upstream Chat and Responses payload tests for native forwarding, inline placement/TTL/count, top-level automatic control, and disabled omission.
- [x] 4.3 Add provider-strategy tests proving unsupported adapters omit only cache metadata, never retry after stripping it, and do not mutate another failover candidate.
- [x] 4.4 Add streaming and non-streaming usage parser tests for OpenAI, Responses, compatible Anthropic counters, Copilot, and Gemini implicit cached input when reported.

## 5. Provider and Usage Implementation

- [x] 5.1 Add and persist typed OpenAI-compatible prompt-cache mode through authentication, configuration update/read APIs, and administration UI types and controls.
- [x] 5.2 Extend shared OpenAI Chat and Responses wire builders for mode-specific native or inline controls while keeping official OpenAI free of Anthropic fields.
- [x] 5.3 Apply explicit omission to every unsupported provider and add payload-free diagnostic metadata for omitted directives.
- [x] 5.4 Centralize cache usage parsing and use it in shared, OpenAI-compatible, Copilot, Azure, Google, and Antigravity non-streaming and streaming paths where their response contracts expose counters.
- [x] 5.5 Update Anthropic, Chat Completions, and Responses response serializers for standard cache-usage fields and stream/non-stream parity.

## 6. Metering and Persistence

- [x] 6.1 Add failing database migration, metering insert/list/aggregate, JSON/API, and unknown-versus-zero tests for uncached/read/write and TTL-specific write counters.
- [x] 6.2 Add the forward-only SQLite migration and extend metering records, asynchronous writes, queries, API types, and administration UI while preserving aggregate input and total token meanings.

## 7. Cache-Aware Affinity

- [x] 7.1 Add failing affinity tests for stable prefix/varying suffix, pre-boundary changes, tools/order/TTL/model/mode/native-key differences, expiry, and legacy unmarked behavior.
- [x] 7.2 Add failing dispatch tests for virtual-upstream and provider-instance preference, successful-only recording, cancellation/failure exclusion, and preserved fallback order.
- [x] 7.3 Implement versioned cacheable-prefix hashing, independently configured five-minute-default expiry, virtual-upstream reordering, provider-instance reordering, and success-gated recording without storing prefix content.

## 8. Deterministic Compatibility and Live Harness

- [x] 8.1 Extend ingestion, provider, serialization, and thin route matrices for all affected API shapes, including five sequential calls/results, terminal continuation, and no sixth call.
- [x] 8.2 Update provider-strategy manifest completeness so every current execution strategy declares supported or concrete not-applicable prompt-cache behavior.
- [x] 8.3 Add a deterministic regression proving provider prompt caching, affinity, and the exact-response cache remain separate and live prompt-cache scenarios cannot be satisfied by a response-cache hit.
- [x] 8.4 Extend the live manifest schema, example, runner, reports, and unit tests with declared prompt-cache capability and two-request cache-read proof for streaming and non-streaming modes.
- [x] 8.5 Run focused Go compatibility packages and `bun test tests/scripts/live-model-matrix.test.ts`, fixing every failure before proceeding.

## 9. Fresh-Port and Native-Client Verification

- [x] 9.1 Run `bun run test:model-matrix:live` without opt-in and verify it exits before credential reads, build, port allocation, state creation, or network access.
- [x] 9.2 Run post-change smoke mode in a fresh isolated process and allocated port; require normalized cache reads greater than zero for every declared-capable row and record sanitized evidence.
- [x] 9.3 Run post-change extended mode in another fresh process and allocated port, verifying five ordered tool cycles, terminal continuation, streams, large results, parallel tools, and cancellation for every applicable row.
- [x] 9.4 Run bounded five-call native Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider smokes with temporary configuration and a fresh gateway port when prerequisites are available; otherwise record each concrete skip reason.
- [x] 9.5 Complete `verification.md` with commit, exact commands, distinct ports, client/provider/model/shape, call/result counts, cache read/write counters, terminal evidence, durations, outcomes, and sanitized skips without payloads or secrets.

## 10. Full Gates, Review, and Archive

- [x] 10.1 Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`.
- [x] 10.2 Run `bun test`, `bun run lint:all`, `bun run typecheck`, `bun run build`, and `bun run spec:check`.
- [x] 10.3 Run code simplification and focused code review on the complete diff, fix verified findings, and rerun every affected check.
- [x] 10.4 Confirm every task and applicable live row is complete, then archive with `bunx openspec archive add-provider-prompt-caching --yes` and verify only archival updates current-state specs.
- [x] 10.5 After archive, rerun `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, `bun run build`, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
