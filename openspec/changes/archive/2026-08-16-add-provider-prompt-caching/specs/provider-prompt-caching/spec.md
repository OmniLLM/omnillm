## Purpose

Defines provider-side prompt prefix reuse controls, cache usage semantics, and the boundary between upstream prompt caching, routing affinity, and OmniLLM response caching.

## ADDED Requirements

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
Provider prompt caching SHALL remain separate from channel affinity and the exact-match response cache: prompt-cache directives SHALL NOT trigger response-cache reads or writes, and a provider prompt-cache hit SHALL still execute upstream inference.

#### Scenario: Prompt control without response-cache control
- **WHEN** a request contains JSON prompt-cache directives but no `X-OmniLLM-Cache` response-cache instruction
- **THEN** prompt-cache processing occurs without changing response-cache eligibility or per-request response-cache behavior
