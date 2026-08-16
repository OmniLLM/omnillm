## Context

See `proposal.md` for motivation. Requests enter through Chat Completions, Anthropic Messages, or Responses ingestion, cross the provider boundary as `cif.CanonicalRequest`, and are rebuilt into provider-specific payloads. Anthropic cache metadata is currently discarded before CIF, top-level system blocks are flattened, and provider usage parsers retain only aggregate tokens. The existing channel-affinity cache uses a conversation-head heuristic, while `internal/lib/responsecache` stores complete canonical responses and is a separate feature.

LiteLLM demonstrates the important interoperability boundary: a frontend can normalize cache intent, but each upstream must receive only the cache protocol it actually supports. Anthropic-style breakpoints, OpenAI-native keys and retention, Bedrock cache points, and Gemini managed cached-content resources are not wire-equivalent. OmniLLM currently has OpenAI Chat/Responses and Gemini-like provider adapters, but no native Anthropic or Bedrock upstream protocol.

## Goals / Non-Goals

**Goals:**

- Make cache intent and usage first-class canonical data rather than route-local raw JSON.
- Preserve cache boundary placement across structural normalization and five-turn agent histories.
- Forward only verified provider-native controls and explicitly omit unsupported metadata without mutating semantic prompt content.
- Expose cache usage consistently to clients and metering.
- Route repeated cacheable prefixes back to the upstream/provider instance that warmed them while retaining failover.
- Prove behavior through fake-upstream tests and isolated fresh-port live evidence.

**Non-Goals:**

- Add a native Anthropic provider, Bedrock Invoke/Converse integration, or Gemini managed `cachedContents` lifecycle.
- Generate automatic breakpoints, pre-warm provider caches, or implement the Anthropic 20-block insertion strategy.
- Add cache price/savings calculation or change token quota policy.
- Change the exact-match response cache or use it as evidence of provider cache reuse.
- Claim cache request support for a provider merely because its response exposes cached-token usage.

## Decisions

### Cache metadata is a first-class CIF contract

Add typed cache control and TTL values to cacheable content parts and tool definitions, plus a request policy containing top-level automatic control and separate OpenAI-native key/retention. Cache placement affects provider-rendered bytes, routing, usage, and tests, so storing it in untyped extensions or retaining raw request maps would create dialect-specific bypasses.

The canonical cache control accepts only `type: ephemeral` with omitted, `5m`, or `1h` TTL. Omitted TTL remains distinct from explicit `5m`. Validation counts all explicit system, message, and tool breakpoints together and caps them at four. Top-level automatic control is not an explicit breakpoint and OmniLLM does not expand it into one.

Alternatives rejected:

- Retaining raw Anthropic JSON in `Extensions`: this would bypass CIF semantics and break on provider failover or cross-dialect serialization.
- One generic string-to-string cache map: it loses placement, TTL omission, and type safety.
- Translating Anthropic markers into OpenAI keys: the providers assign different meanings and invalidation behavior.

### Structured system blocks become authoritative

Replace the authoritative flattened system string with ordered canonical system blocks carrying optional cache control. Convert every ingestion path to that representation and provide a shared plain-text renderer only for provider adapters that cannot represent structured blocks. Mid-conversation system entries remain in ordered messages instead of being hoisted.

This migration is broader than adding one field, but preserving the exact prefix is impossible after block flattening. Keeping both a mutable `SystemPrompt` and `SystemBlocks` indefinitely was rejected because they can diverge and make cache keys nondeterministic.

### Provider request behavior is explicit and conservative

OpenAI-compatible instances gain a persisted `prompt_cache_mode`:

- `auto`: `openai_native` only for `api.openai.com`, otherwise `disabled`.
- `disabled`: omit all request-side prompt-cache metadata.
- `openai_native`: omit Anthropic markers and forward supplied `prompt_cache_key` and `prompt_cache_retention` on supported Chat/Responses payloads.
- `anthropic_inline`: for explicitly compatible custom gateways, emit canonical `cache_control` at its preserved system, content, tool-result, tool, or top-level location.

`anthropic_inline` is invalid for official OpenAI. Existing custom endpoints behave as disabled, avoiding new upstream 400s. Unsupported provider adapters omit only cache metadata and emit payload-free diagnostic fields; they do not retry after stripping metadata.

Official OpenAI/Codex forwarding follows the documented native contract. OpenAI OAuth, Copilot, Azure, Alibaba, Kimi, ModelScope, Google, and Antigravity remain explicit omission unless baseline provider evidence verifies their request contract. Response parsers may still record standard cache-read usage without implying request-control support.

Alternatives rejected:

- Forwarding all unknown fields to OpenAI-compatible endpoints: many reject Anthropic extensions.
- Adding a native Anthropic adapter inside this change: that requires new authentication, discovery, model, HTTP, streaming, and provider lifecycle work unrelated to preserving cache semantics for current providers.
- Implementing Gemini explicit caching: managed resource creation, deterministic lookup, TTL vocabulary, cleanup, and platform differences are a separate control-plane feature.

### Usage has a normalized total and optional disjoint detail

`CIFUsage.InputTokens` remains total prompt input so existing metering and OpenAI serializers preserve their meaning. Add optional uncached input, cache-read input, cache-write input, and 5m/1h write detail with shared constructors/validators:

- OpenAI/Gemini totals already include cached reads; uncached input is total minus a valid cached subset.
- Anthropic-style input is the uncached remainder; canonical total is uncached plus reads plus writes.
- Missing write counts or TTL detail remain unknown and are never inferred.

Anthropic output reconstructs its exclusive buckets; Chat and Responses expose total input plus standard nested cached-token details. Streaming and non-streaming use the same helpers. Cache usage gets dedicated request-log columns, while existing aggregate input/total fields keep their meanings.

Alternatives rejected:

- Reinterpreting existing `InputTokens` as uncached input: this would silently corrupt historical metrics and OpenAI totals.
- Storing unknown as zero in CIF: callers could not distinguish a reported zero hit from absent provider detail.
- Adding cost savings now: current metadata lacks complete provider/model/TTL write pricing.

### Affinity keys the effective cacheable prefix

For marked requests, render the canonical prompt in provider order (`tools`, top-level system, ordered messages), stop at the final effective explicit boundary, include model, selected cache mode, TTLs, optional configured user identity, and native key, and store only a versioned digest. Content after the boundary does not affect the key. Top-level automatic control uses the last cacheable canonical element as its effective boundary for affinity only; it remains automatic on the provider request.

Without prompt-cache directives, retain the existing stable-head key behavior to avoid changing unrelated routing. Prompt-cache affinity first prefers a prior eligible virtual upstream, then its provider instance. It records only after successful execution or a successfully started stream, never after pre-response failure or cancellation. Affinity TTL defaults to five minutes and is configured independently of provider TTL.

Alternatives rejected:

- Hashing the full request: a varying suffix would defeat prefix reuse.
- Reusing the response-cache key: it intentionally includes complete generation semantics and serves a different purpose.
- Mirroring a one-hour provider TTL: stale routing pins can outlive provider health and load-balancing needs.

### Live evidence uses usage, not latency

The live matrix adds a declared prompt-caching capability and sends two sequential requests through the same model, provider instance, and upstream shape. Their sufficiently large canonical prefix is byte-equivalent through the boundary and their suffix differs. The exact-response cache is disabled. A declared-capable row passes only when the second response reports normalized cache-read tokens greater than zero; lower latency alone is not evidence. Missing usage support is skipped or not applicable with a concrete reason, not passed.

Every baseline and post-change suite starts a separate isolated process that allocates `127.0.0.1:0`, and the report records the actual port. Native coding clients use temporary HOME/config roots when safely possible and otherwise receive concrete skips; deterministic five-call shape coverage is always required.

## Risks / Trade-offs

- **[Risk] Structured system migration changes provider prompts** → Preserve order and existing join separators at incapable adapter boundaries; add golden payload tests for every strategy.
- **[Risk] Custom endpoints reject cache extensions** → Default to `disabled` and require explicit `anthropic_inline` opt-in.
- **[Risk] A provider uses different usage total semantics** → Centralize constructors for total-subset and exclusive-bucket shapes and test malformed counters.
- **[Risk] Stream-final usage is emitted separately from terminal content** → Accumulate provider usage-only frames and emit exactly one dialect terminal sequence with final usage.
- **[Risk] Candidate transformation aliases mutable slices/maps** → Deep-copy cache-bearing canonical data per candidate and test capable-to-incapable failover.
- **[Risk] Affinity changes virtual-model balancing** → Reorder only an eligible recorded upstream, preserve all relative fallback order, expire quickly, and retain legacy behavior for unmarked requests.
- **[Risk] Cacheable prefixes or keys leak through logs/evidence** → Persist only digests/counters and test logs for metadata-only fields.
- **[Risk] Exact-response cache masks live proof** → Disable it in prompt-cache scenarios and reject response-cache hit headers as provider-cache evidence.
- **[Risk] Provider minimum cache size varies** → Configure live rows with provider/model-specific sufficiently large stable prefixes; do not guess a universal production tokenizer threshold.
- **[Risk] Scope expands into unsupported provider protocols** → Keep native Anthropic/Bedrock and Gemini managed caching as named follow-up changes.

## Migration Plan

1. Strictly validate and obtain human approval for this change.
2. Capture baseline deterministic and fresh-port live evidence using existing behavior.
3. Add failing deterministic tests for CIF, ingestion, providers, usage, metering, affinity, and client-shape contracts.
4. Migrate CIF/system representations, then ingestion and provider adapters, then usage/persistence and affinity.
5. Extend the live matrix and record post-change fresh-port and applicable native-client evidence.
6. Run focused checks, the complete Bun/Go suite, code review, and `spec:check`.
7. Check tasks and archive only after all required evidence passes; rerun all gates after archive.

Rollback removes the cache-specific fields, provider mode, usage columns from active consumers, and affinity v2 selection while preserving the forward-only database migration columns. Existing configurations without `prompt_cache_mode` already resolve to safe defaults, so rollback does not require rewriting provider records.
