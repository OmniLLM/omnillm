## ADDED Requirements

### Requirement: OpenAI-compatible prompt-cache modes
An OpenAI-compatible provider instance SHALL support persisted prompt-cache modes `auto`, `disabled`, `openai_native`, and `anthropic_inline`; `auto` SHALL resolve official `api.openai.com` endpoints to `openai_native` and all other endpoints to `disabled`.

#### Scenario: Existing custom endpoint
- **WHEN** an existing custom OpenAI-compatible provider has no prompt-cache mode configured
- **THEN** it remains usable and omits prompt-cache request metadata by default

#### Scenario: Official OpenAI safety
- **WHEN** an operator attempts to configure `anthropic_inline` for an official OpenAI endpoint
- **THEN** configuration is rejected before a request can send unsupported fields upstream

### Requirement: Native OpenAI request controls
Official OpenAI-compatible Chat Completions and Responses requests in native mode SHALL omit Anthropic-style `cache_control` fields and SHALL forward supplied native prompt cache key and retention values on supported upstream shapes.

#### Scenario: Anthropic inbound request reaches official OpenAI
- **WHEN** an Anthropic Messages request carrying explicit breakpoints routes to official OpenAI without native OpenAI hints
- **THEN** the OpenAI payload omits the markers and retains the same prompt content and tool history

#### Scenario: Native cache hints reach official OpenAI
- **WHEN** a Chat Completions or Responses request supplies native cache key or retention values and routes to official OpenAI
- **THEN** the matching upstream request includes those values unchanged

### Requirement: Opt-in compatible inline controls
A custom OpenAI-compatible instance configured for `anthropic_inline` SHALL preserve cache controls at their canonical system, message-content, tool-result, tool-definition, and top-level placements without reordering prompt content or adding breakpoints.

#### Scenario: Compatible inline payload
- **WHEN** a marked request routes to an explicitly inline-compatible custom instance
- **THEN** the upstream payload contains the same number, TTLs, relative placements, and ordering of cache controls represented by CIF

### Requirement: Unsupported-provider omission
Providers without a verified request-control contract SHALL omit prompt-cache directives while preserving all generation-affecting content and SHALL NOT retry after removing cache controls in response to an upstream error.

#### Scenario: Unsupported provider rejects request for another reason
- **WHEN** a provider receives a prompt whose cache metadata was intentionally omitted and the upstream returns an error
- **THEN** existing error and failover behavior applies without a second cache-stripped retry

### Requirement: Provider cache usage parsing
Provider adapters SHALL normalize standard cached-input counters from Chat Completions, Responses, and Gemini usage metadata when those counters are present in non-streaming or final streaming usage.

#### Scenario: Implicit Gemini cache read
- **WHEN** a Gemini response reports `cachedContentTokenCount`
- **THEN** canonical usage records that value as cache-read input without claiming that OmniLLM created a managed cached-content resource

#### Scenario: Missing cache detail
- **WHEN** a provider reports aggregate input usage without a cache counter
- **THEN** the adapter preserves aggregate input and leaves cache detail unknown rather than reporting a zero-valued hit

### Requirement: Deferred provider cache protocols
The system SHALL NOT claim native Anthropic, Bedrock cache-point, or Gemini managed cached-content request support until the corresponding upstream provider protocol and lifecycle are implemented and tested.

#### Scenario: Provider catalog lacks deferred protocol
- **WHEN** provider capabilities are reported for the current release
- **THEN** native Anthropic, Bedrock, and Gemini managed-cache request support are absent rather than inferred from inbound dialect compatibility
