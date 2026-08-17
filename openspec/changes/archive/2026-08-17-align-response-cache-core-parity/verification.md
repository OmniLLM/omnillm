## Summary

Implemented core LiteLLM-compatible exact-response-cache behavior while retaining OmniLLM's opt-in control, canonical CIF replay, Redis namespace/schema, and provider prompt-cache separation.

## Deterministic and build verification

| Command | Result |
|---|---|
| `bun run spec:validate` | Passed: 15 items, 0 failures |
| `bun run spec:check` | Passed strict validation and mandatory spec gate |
| Focused Go cache/client matrix | Passed for responsecache, routes/server, ingestion, serialization, database, commands, providers/shared, and testcompat |
| `OMNILLM_TEST_REDIS_URL=redis://127.0.0.1:6379/0 go test -race ./internal/lib/responsecache -run TestRealRedisResponseCacheSmoke -count=1` | Passed against local Redis; covered native expiry, hits, namespace isolation, clear, and unrelated-key preservation |
| `go vet ./...` | Passed |
| `go build ./...` | Passed |
| `go test -race ./...` | Passed all packages |
| `bun run lint:all` | Passed; only upstream baseline-browser-mapping freshness warning |
| `bun run typecheck` | Passed |
| `bun test` | Passed: 361 tests, 22 expected skips, 0 failures |
| `bun run build` | Passed; only existing Vite chunk-size warning |
| `git diff --check` | Passed |

One full `internal/server` run transiently failed the pre-existing provider-rename priority test (`expected 7, got 0`); its isolated rerun and the subsequent complete focused server suite both passed.

## Compatibility evidence

Deterministic regressions cover Claude Code Messages, Copilot-compatible Chat, Codex Responses custom `exec`, and Droid Responses custom `ApplyPatch` shapes. Each fixture carries five ordered tool calls/results, preserves identifiers, kinds, raw input, namespaces and structured results, reaches terminal continuation, and rejects an unexpected sixth call. Route tests prove first miss/second hit with exactly one upstream execution. Responses stream tests prove native custom input deltas, one terminal event, and no function-argument events for custom tools.

### Native client smokes

All smokes used bounded commands and sanitized evidence. Claude Code, Codex, and Droid targeted an isolated fresh-port gateway at `127.0.0.1:39146` backed by Redis prefix `omnillm-native-39146`; Copilot CLI lacked a configured OmniLLM custom-provider model and therefore exercised its authenticated native model rather than the isolated gateway.

| Client | Version | Shape/tool | Calls | Results | Terminal | Exit | Exact-cache evidence | Outcome |
|---|---|---|---:|---:|---|---:|---|---|
| Claude Code | 2.1.233 | Anthropic Messages / Bash | 5 | 5 | `CLAUDE_CACHE_FIVE_CALL_PASS` | 0 | Repeated six-turn runs completed; provider prompt-cache reads observed. No identical final-request response-cache entry remained after the tool loop | Pass for required native tool loop; exact-cache hit not applicable to changing multi-turn requests |
| Codex CLI | 0.147.0 | Responses / command execution | 5 | 5 | `CODEX_CACHE_FIVE_CALL_PASS` | 0 | Isolated gateway Responses path; requests changed after each tool result, so no identical request pair | Pass for required native tool loop; exact-cache hit not applicable to changing multi-turn requests |
| Droid | 0.197.0 | Responses custom provider / execute | 5 | 5 inferred from six turns | `DROID_CACHE_FIVE_CALL_PASS` | 0 | Isolated gateway custom Responses model; output reported provider cache-read input, while deterministic route tests separately prove exact miss/hit | Pass for required native tool loop |
| GitHub Copilot CLI | 1.0.81-0 | Native shell | 5 | 5 observed in output markers | `COPILOT_CACHE_FIVE_CALL_PASS` | 0 | No safe temporary OmniLLM custom-provider model was configured; direct custom-provider smoke skipped for this concrete reason | Native loop pass; OmniLLM custom-provider live shape skipped |

Exact response-cache miss/hit and stream evidence is deterministic and real-Redis backed because native agent loops necessarily mutate conversation history between calls and do not produce identical request keys.

## Live model matrix

| Command | Result |
|---|---|
| `OMNILLM_RUN_LIVE_MATRIX=0 bun run test:model-matrix:live:smoke` | Safe no-op passed before credential/network work |
| `OMNILLM_RUN_LIVE_MATRIX=1 OMNILLM_LIVE_MATRIX_MODE=smoke ...` | 0 failed; 21 skipped for missing `OMNILLM_LIVE_GITHUB_TOKEN`; 3 not applicable with declared reasons |
| `OMNILLM_RUN_LIVE_MATRIX=1 OMNILLM_LIVE_MATRIX_MODE=extended ...` | 0 failed; 39 skipped for missing `OMNILLM_LIVE_GITHUB_TOKEN`; 27 not applicable with declared reasons |

The extended harness now passes an explicit Redis URL and per-run unique prefix, disables exact caching outside its dedicated scenario, and verifies both non-streaming and streaming miss/hit pairs with terminal uniqueness. Credentialed provider rows could not execute because `OMNILLM_LIVE_GITHUB_TOKEN` was not set; this is recorded as skip rather than success.

## Review and resolved findings

- Extracted route cache lifecycle into `internal/routes/response_cache.go` and retained transport-only SSE helpers.
- Reused one Responses SSE formatter for live and cached paths.
- Reused one stream-finalization helper across all three routes.
- Avoided the second TTL query when disabled during the initial optimization, then restored independent persisted-TTL visibility after review.
- Added forwarded user identity to semantic cache keys to prevent cross-user replay.
- Sanitized provider prompt-cache counters before wire replay and hit metering.
- Added custom-tool stream-origin regression coverage.
- Made cached SSE replay cancellation/write-error aware so unsuccessful delivery is not logged or metered as a served hit.
- Added explicit isolated Redis configuration and streaming proof to the live exact-cache scenario.

Deferred quality-only architectural suggestions, because they would expand this approved change beyond behavior-preserving cleanup:

- Moving CIF stream accumulation/synthesis out of `responsecache` into a broader CIF codec.
- Moving semantic key projection ownership into CIF.
- Replacing the cache payload codec with a new general CIF decode API.
- Reworking exact-response live capability from per-row declarations to a new suite-level manifest contract.

## Known trade-offs

- Sampling-independent caching intentionally replays a prior stochastic result within TTL; caching remains opt-in and supports bypass/off controls.
- Concurrent identical misses are not coalesced.
- Canonical replay preserves semantics, identifiers and tool relationships, but not original wire bytes, chunk boundaries or event timing.
- Cached stream replay preflights serialization before committing the hit response, trading additional memory/TTFB for safe upstream fallthrough when conversion fails.
