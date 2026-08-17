## Why

OmniLLM's opt-in exact-response cache currently excludes Responses API requests, requires deterministic sampling parameters, and defaults entries to one hour. Aligning its core behavior with LiteLLM will make caching consistent across OmniLLM's three supported generation dialects and useful for coding-agent requests that commonly omit `temperature`, while keeping upstream provider prompt caching visibly and operationally separate.

## What Changes

- Keep exact-response caching opt-in, but make valid Chat Completions, Anthropic Messages, and Responses requests eligible regardless of `temperature` or `top_p`.
- Change the fallback/default response-cache TTL from one hour to 60 seconds without modifying existing positive operator-configured TTLs.
- Add canonical non-streaming and streaming cache population and replay for `/v1/responses`, including lossless native custom-tool metadata and input deltas.
- Harden semantic cache identity for Responses state and generation-affecting controls while excluding transport, dialect, routing, and provider prompt-cache placement metadata.
- Treat cache-key serialization failure as ineligible instead of mapping unrelated requests to a fallback digest.
- Record successfully served exact-response hits as response-cache traffic without fabricating upstream provider prompt-cache hits or misses.
- Preserve the current OmniLLM Redis namespace, hash schema, canonical cross-dialect replay, per-request controls, fail-open behavior, and provider prompt-cache processing.
- Add deterministic and bounded live compatibility evidence for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider request shapes.
- Document that sampling-independent caching can replay a prior stochastic result during the TTL and that OmniLLM's Redis format is not LiteLLM wire/storage compatible.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `caching`: Broaden exact-response eligibility and route scope, change the default TTL, harden semantic identity, and define Responses replay and hit accounting.
- `gateway-api`: Expose correct `/v1/responses` hit, miss, fallthrough, streaming-terminal, and native custom-tool behavior.
- `cif-translation`: Preserve all function/custom tool metadata and custom input deltas through canonical cache encode, accumulation, and replay.
- `provider-prompt-caching`: Keep provider cache directives, affinity, and usage independent from exact-response cache identity and accounting.
- `persistence`: Record exact-response cache hits without presenting historical cached provider counters as new provider prompt-cache activity.
- `model-compatibility-testing`: Verify cache population/replay across all maintained coding-agent shapes, including five sequential tool calls and bounded live evidence.

## Impact

- **Runtime:** Requests with omitted or nonzero sampling controls become cacheable when the operator-enabled cache is active; repeated requests may receive a prior stochastic output within the configured TTL.
- **Public API:** `/v1/responses` gains the existing `X-OmniLLM-Cache: hit|miss` behavior for non-streaming and streaming requests. Existing bypass/off values remain unchanged.
- **Storage:** Redis key format, namespace, hash fields, and schema version remain unchanged. The canonical JSON payload gains backward-compatible optional custom-tool fields. No SQLite migration is required.
- **Code:** `internal/lib/responsecache`, Chat/Messages/Responses routes, metering helpers, compatibility fixtures/tests, and cache documentation are affected.
- **Operations:** The fallback TTL changes to 60 seconds. Positive persisted TTL values continue to win, and Redis remains optional and fail-open.
- **Out of scope:** LiteLLM Redis key/value compatibility, semantic caching, distributed coordination or affinity, miss coalescing, UI redesign, embeddings caching, and new API routes.
