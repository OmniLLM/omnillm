## Why

OmniLLM currently discards provider prompt-cache controls during request ingestion, flattens structured system content that defines cache boundaries, and ignores provider-reported cache usage. Multi-turn coding agents therefore lose cache intent and observability even when an upstream can reuse an identical prompt prefix, increasing latency and input cost and weakening provider-instance affinity.

## What Changes

- Add first-class prompt-cache controls to CIF for Anthropic breakpoints and automatic caching, while retaining OpenAI-native prompt cache key and retention as separate semantics.
- Preserve structured system blocks and supported mid-conversation system messages instead of flattening or hoisting them before dispatch.
- Validate Anthropic cache-control type, TTL, legal placement, and the four-breakpoint limit with dialect-specific client errors.
- Add capability-gated request forwarding for OmniLLM's current providers: official OpenAI-native controls, opt-in inline markers for compatible custom OpenAI endpoints, and deliberate metadata omission for unsupported providers.
- Normalize cache-read, cache-write, uncached, and TTL-specific usage across non-streaming and streaming paths and persist those counters in metering.
- Make channel affinity key explicit cacheable prefixes and prefer the upstream/provider instance that successfully warmed that prefix without removing failover choices.
- Extend deterministic client/provider matrices and the isolated live matrix to prove cache reuse, five-turn tool continuity, and no regressions on automatically allocated ports.
- Keep OmniLLM's exact-match response cache distinct and unchanged. Native Anthropic/Bedrock provider support, Gemini managed cached-content lifecycle, gateway-generated breakpoints, prompt-cache pricing, and cache pre-warming are explicitly out of scope.

## Capabilities

### New Capabilities

- `provider-prompt-caching`: Defines canonical prompt-cache controls, provider-specific request handling, cache usage accounting, and separation from full-response caching.

### Modified Capabilities

- `cif-translation`: Preserve structured system content, cache controls, native cache hints, and normalized cache usage through canonical translation.
- `gateway-api`: Accept and validate dialect-specific prompt-cache controls and return cache usage consistently.
- `providers`: Apply explicit provider capability/configuration policies for forwarding, omitting, and parsing prompt-cache data.
- `routing-failover`: Use cacheable-prefix affinity across virtual upstreams and provider instances while retaining failover.
- `persistence`: Store cache usage buckets without changing aggregate input-token meaning.
- `model-compatibility-testing`: Add deterministic and credential-gated live prompt-cache scenarios while preserving all coding-agent tool-loop requirements.

## Impact

This changes the public Chat Completions, Anthropic Messages, and Responses request/usage contracts; CIF request and usage structures; OpenAI-compatible provider configuration and payload transforms; provider response parsers; channel-affinity keys and virtual-model ordering; SQLite request-log schema and consumers; shared compatibility fixtures; and live-matrix schema, runner, reports, and tests. Existing provider configurations remain compatible because custom endpoints default to disabled prompt-cache forwarding, legacy unmarked requests retain their current semantic payloads, and database migration preserves existing aggregate counters.
