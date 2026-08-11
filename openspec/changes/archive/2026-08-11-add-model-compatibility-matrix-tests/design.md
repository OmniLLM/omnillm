## Context

See `proposal.md` for motivation. OmniLLM currently has strong unit coverage for individual ingestion and serialization paths, several route-level dialect tests, and a partial provider-shape matrix. However, the existing provider matrix covers only a subset of provider types, initial tool generation and repeated tool loops are not matrix-wide, streaming and timeout strategies are tested unevenly, and the existing live script uses one API shape and normal user configuration.

The design must cover three inbound generation dialects, ten provider types, models that select different upstream APIs, native and locally synthesized streaming, and agentic tool histories without creating a brittle product of every dimension. Live testing must be useful when credentials are available but safe and non-blocking when they are not.

## Goals / Non-Goals

**Goals:**

- Prove each client API shape preserves the same canonical semantics.
- Prove each provider execution strategy maps canonical requests and events to its supported upstream contract.
- Retain a thin end-to-end cross-shape/provider matrix for wiring confidence.
- Exercise long, repeated, parallel, and error-bearing tool loops deterministically.
- Provide isolated opt-in live smoke and extended matrices with explicit capability declarations and outcome reporting.
- Make planned coverage auditable so new provider/model strategies cannot be added without a matrix row.

**Non-Goals:**

- Execute every advertised upstream model in required CI.
- Assert exact generated prose, latency rankings, token counts, or nondeterministic model choices.
- Treat unsupported tool, stream, or API-shape combinations as failures.
- Read or mutate the user's normal OmniLLM database during live tests.
- Change production routing, provider, translation, timeout, or credential behavior solely to make tests easier.
- Store real credentials or token bundles in repository files or test reports.

## Decisions

### 1. Use a layered matrix rather than a full Cartesian product

The deterministic suite will have three complementary layers:

1. **Dialect contract layer:** every semantic fixture passes through all three ingestion and serialization dialects in streaming and non-streaming modes. Assertions compare canonical meaning and dialect vocabulary.
2. **Provider contract layer:** each provider execution strategy receives equivalent canonical requests and is checked against a fake upstream for endpoint, model, turn, tool, streaming, timeout, and cancellation behavior.
3. **End-to-end route layer:** a thinner set runs each inbound shape through representative provider/model rows to catch route, registry, model-resolution, and serialization wiring errors.

This separates proof of `shape -> CIF` from `CIF -> provider` while retaining enough full-stack coverage to detect integration errors.

**Alternative considered:** every shape × provider × model × stream × scenario combination. Rejected because duplicate evidence would make the suite slow, hard to diagnose, and expensive to maintain while still missing semantic edge cases.

### 2. Maintain behavior-oriented model rows

A test manifest will identify provider type, representative model identifier, upstream execution shape, streaming strategy, and supported capabilities. Dynamic providers will have separate rows for each meaningful strategy, such as Copilot Chat Completions, Copilot Responses, Copilot Claude streaming, and OpenAI-compatible Chat Completions and Responses.

Deterministic rows may use stable behavior-oriented fixture model IDs where marketing versions are irrelevant. Tests for provider-specific model quirks retain exact known model identifiers only when model selection changes behavior.

The matrix will fail if a required deterministic strategy row is absent. Unsupported combinations must be declared as not applicable rather than inferred from a failed request.

### 3. Share canonical semantic fixtures

A test-only fixture package will define canonical scenarios and dialect request builders for:

- plain text generation;
- one tool call and result replay;
- at least three sequential tool cycles;
- parallel calls with interleaved streamed arguments;
- mixed text and tool calls;
- required and optional empty arguments;
- large tool results;
- errored tool results;
- thinking before tool use;
- normal, abrupt, and cancelled streams.

Assertions will focus on stable structure: call identity, argument JSON, result association, turn ordering, terminal event vocabulary, canonical equivalence, and absence of duplicate events. Timestamps and model-generated wording will not be compared exactly.

**Alternative considered:** duplicate request literals in each package. Rejected because equivalent scenarios would drift across dialects and providers.

### 4. Simulate long-running behavior deterministically

Deterministic long calls and streams will use controllable channels, fake clocks where existing seams allow them, context cancellation, and injected short test budgets. They will not sleep for production timeout durations.

The tests will distinguish:

- time to response headers;
- total non-stream response budget;
- stream body lifetime after headers;
- caller cancellation;
- provider retry or failover eligibility before versus after output.

This exercises long agentic tasks without making CI slow or flaky.

### 5. Build an explicit isolated live runner

A Bun/TypeScript runner will require `OMNILLM_RUN_LIVE_MATRIX=1` before doing work. It will accept a manifest path and modes `smoke` or `extended`, create a temporary HOME/config/database, build and launch the actual binary on an automatically allocated loopback port, provision only declared provider rows, execute bounded requests, write sanitized JSON results under an ignored temporary directory, and shut down the process and remove temporary state.

Each row references credential environment-variable names or an explicitly supplied isolated token-bundle path; manifests never contain secret values. OAuth rows are skipped unless an isolated token bundle is explicitly supplied. The runner will never silently reuse the normal OmniLLM database.

Every request will have an abort timeout, with separate startup, request, per-model, and suite budgets. Console output and reports will include row identity and sanitized errors but no authorization headers, API keys, token bundles, request bodies containing tool results, or raw upstream payloads.

**Alternative considered:** extend the existing fixed-port script while continuing to use normal configuration. Rejected because it can mutate or expose user state, cannot classify unavailable credentials cleanly, and covers only Chat Completions.

### 6. Separate smoke and extended live evidence

Smoke mode will run each declared compatible API shape for plain non-stream, plain stream, and one forced tool-call/result replay where supported. Extended mode additionally runs repeated tool cycles, parallel tools, large results, long-lived streams, and cancellation.

A row with missing credentials is `skipped`; an explicitly unsupported capability is `not_applicable`; a row with supplied credentials that cannot authenticate or execute is `failed`. The command exits nonzero for failures or missing planned deterministic coverage, but not solely for skips or not-applicable rows.

Live tests remain outside required CI because of credentials, cost, quota, latency, and provider availability. Deterministic matrix tests become part of normal verification.

## Risks / Trade-offs

- **[Risk] The matrix becomes too large and slow** → Layer proofs, share fixtures, and keep exhaustive provider/live combinations out of required CI.
- **[Risk] Representative models stop matching provider behavior** → Declare execution strategy and capabilities in maintained manifests and fail deterministic tests when planned rows disappear.
- **[Risk] Live tests consume cost or quota unexpectedly** → Require explicit opt-in, expose smoke/extended modes, bound each request and suite, and report planned rows before execution.
- **[Risk] Credentials or tool results leak into output** → Reference secrets only by environment name, sanitize errors, isolate runtime state, and test report redaction with synthetic sentinels.
- **[Risk] Live provider nondeterminism causes flakes** → Assert envelopes, tool identity, valid arguments, terminal events, and accepted result replay rather than exact prose.
- **[Risk] Unsupported capabilities hide regressions** → Require explicit not-applicable declarations and distinguish them from missing or failed rows.
- **[Trade-off] Live coverage is not a merge gate** → Keep deterministic fake-upstream coverage comprehensive and publish live reports for release or operator-triggered verification.

## Migration Plan

1. Add shared deterministic fixtures and migrate/extend existing tests without removing established coverage.
2. Complete dialect and provider strategy manifests, then make missing deterministic rows fail.
3. Expand route integration for streaming and multi-turn agentic cases.
4. Add and document the isolated opt-in live runner while retaining the existing live command as a temporary compatibility alias if needed.
5. Add deterministic matrix commands to normal verification; keep live commands manual.

No production data migration or rollout is required. Rollback removes the new fixtures, matrix tests, scripts, and package commands; production binaries and stored configuration remain unchanged.
