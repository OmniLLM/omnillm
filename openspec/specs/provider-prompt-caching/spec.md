# provider-prompt-caching Specification

## Purpose
Defines provider-side prompt prefix reuse controls, cache usage semantics, and the boundary between upstream prompt caching, routing affinity, and OmniLLM response caching.
## Requirements
### Requirement: Canonical prompt-cache directive vocabulary
The system SHALL represent an explicit cache breakpoint as `type: ephemeral` with an optional TTL of `5m` or `1h`, SHALL distinguish omitted TTL from an explicit TTL, and SHALL represent top-level automatic caching separately from explicit breakpoints.

#### Scenario: Default cache lifetime
- **WHEN** a caller supplies an ephemeral breakpoint without a TTL
- **THEN** canonical form preserves an omitted TTL rather than inventing an explicit duration

#### Scenario: Extended cache lifetime
- **WHEN** a caller supplies an ephemeral breakpoint with TTL `1h`
- **THEN** canonical form preserves the explicit one-hour TTL

### Requirement: Provider-native controls remain distinct
The system SHALL preserve OpenAI-native `prompt_cache_key` and `prompt_cache_retention` independently from Anthropic-style cache breakpoints and SHALL NOT synthesize one control family from the other.

#### Scenario: Request contains both control families
- **WHEN** a canonical request contains an Anthropic breakpoint and an OpenAI-native cache key
- **THEN** each provider adapter independently forwards or omits each family according to its verified contract

### Requirement: Explicit provider handling
Every provider adapter SHALL explicitly forward, translate, or omit prompt-cache directives according to its configured and verified upstream capability, and omission SHALL leave prompt text, content order, tools, calls, and results unchanged.

#### Scenario: Unsupported provider receives marked request
- **WHEN** a request containing cache directives routes to a provider without a verified compatible request contract
- **THEN** the provider receives the same semantic prompt without the unsupported cache metadata

### Requirement: Canonical cache usage accounting
The system SHALL distinguish total prompt input, uncached input, cache-read input, cache-write input, and known five-minute and one-hour cache-write input, and SHALL NOT fabricate a bucket a provider did not report.

#### Scenario: OpenAI cached subset
- **WHEN** an OpenAI-shaped response reports total input and cached input as a subset
- **THEN** canonical total remains the reported total and uncached input equals total minus cached input

#### Scenario: Anthropic exclusive buckets
- **WHEN** an Anthropic-shaped response reports uncached, cache-read, and cache-write input separately
- **THEN** canonical total input equals the sum of those disjoint buckets

### Requirement: Prompt-cache mechanism separation
Provider prompt caching SHALL remain separate from channel affinity and the exact-response cache: prompt-cache directives SHALL NOT enable, disable, read, write, or change the semantic identity of the exact-response cache; a provider prompt-cache hit SHALL still execute upstream inference unless an independently enabled exact-response entry is served; and an exact-response hit SHALL NOT be reported as provider prompt-cache activity.

#### Scenario: Prompt control without response-cache control
- **WHEN** a request contains JSON prompt-cache directives but no exact-response cache instruction
- **THEN** prompt-cache processing occurs without changing response-cache enablement or per-request response-cache behavior

#### Scenario: Semantically identical prompt-cache placement
- **WHEN** two otherwise semantically identical requests differ only in prompt-cache breakpoint placement, native cache key, or retention hints
- **THEN** they share exact-response semantic identity while retaining their distinct provider controls on an exact-response miss

#### Scenario: Provider cache hit after exact-response miss
- **WHEN** no exact-response entry is served and the upstream provider reports cached input
- **THEN** the request is recorded as provider prompt-cache activity from an upstream execution

#### Scenario: Exact-response hit
- **WHEN** the gateway serves an exact-response cache entry without upstream execution
- **THEN** it does not infer or report a new provider prompt-cache hit, miss, read, or write

