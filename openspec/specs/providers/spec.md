# providers Specification

## Purpose
Defines supported upstream provider types and instances, credential persistence, model discovery, execution, and provider compatibility behavior exposed through OmniLLM.
## Requirements
### Requirement: Provider type catalog
The system SHALL support distinct provider types for GitHub Copilot, Antigravity, Alibaba, ModelScope, Azure OpenAI, Google, Kimi, OpenAI-compatible endpoints, Codex API keys, and OpenAI OAuth accounts.

#### Scenario: Distinct OpenAI modes
- **WHEN** provider types are listed
- **THEN** API-key Codex and OAuth OpenAI accounts remain distinct types

### Requirement: Provider instance registration
Every provider SHALL have a unique instance identifier independent of provider type, and registering the same identifier SHALL replace its previous in-process registration.

#### Scenario: Unknown instance
- **WHEN** an unknown instance is requested
- **THEN** lookup fails and reports available instance identifiers

### Requirement: Active provider selection
The registry SHALL maintain active instances and a primary active provider, promoting another active instance if the primary is removed.

#### Scenario: First activation
- **WHEN** the first instance is activated
- **THEN** it becomes the primary provider

### Requirement: Credential and configuration persistence
Provider authentication SHALL persist per-instance credentials and configuration so providers can be reconstructed after restart.

#### Scenario: OpenAI-compatible setup
- **WHEN** a valid endpoint and optional API key are configured
- **THEN** the normalized base URL, authentication type, display data, and token are stored for that instance

### Requirement: Secure compatible endpoints
OpenAI-compatible endpoints SHALL require an HTTP or HTTPS base URL, reject DashScope misclassification, and pass endpoint security validation before persistence and each model-discovery call.

#### Scenario: Invalid stored endpoint
- **WHEN** a stored endpoint fails security validation before model discovery
- **THEN** no upstream network request is made

### Requirement: Cached model discovery
Provider model lists SHALL use a per-instance and provider-type cache with a 24-hour default TTL, while forced refresh SHALL bypass it.

#### Scenario: Provider type changes
- **WHEN** an instance identifier is reused for a different provider type
- **THEN** the old provider-type cache is not used

### Requirement: Model state overlay and degradation
Persisted enablement SHALL be applied to cached and fresh models, unknown states SHALL default enabled, duplicates SHALL collapse, and discovery failure SHALL fall back to persisted states when available.

#### Scenario: Discovery fails with stored states
- **WHEN** upstream discovery fails and model states exist
- **THEN** a model list is built from those stored identifiers and enablement values

### Requirement: Tool failures remain visible
A provider or proxy failure for a request containing tools SHALL return the underlying error and SHALL NOT retry the request after silently removing tools.

#### Scenario: Tool request fails
- **WHEN** upstream tool execution fails
- **THEN** the client receives the actual failure and no plain-chat retry is issued

### Requirement: Provider compatibility transforms
Provider adapters SHALL preserve gateway contracts while applying verified provider-specific execution and serialization rules.

#### Scenario: Alibaba client streaming
- **WHEN** a streaming request routes to Alibaba
- **THEN** the upstream call executes non-streaming and the canonical result is re-streamed locally

#### Scenario: DeepSeek V4 tool turn
- **WHEN** an Alibaba DeepSeek V4 request includes tools
- **THEN** upstream thinking is disabled and upstream `tool_choice` is omitted

#### Scenario: OpenAI-compatible tool history
- **WHEN** an assistant history message contains only tool calls
- **THEN** OpenAI-compatible serialization emits empty-string content with the tool calls

#### Scenario: Copilot GPT-5 family
- **WHEN** Copilot receives `gpt-5-mini`
- **THEN** it uses chat completions, while other GPT-5-family models may use Responses unless chat shape is forced

#### Scenario: Copilot tool-call stream
- **WHEN** Copilot interleaves indexed tool-call arguments or supplies first-chunk arguments
- **THEN** identifiers and arguments are accumulated by provider index and terminal stop is upgraded to tool use

#### Scenario: User identifier cap
- **WHEN** an OpenAI-compatible user identifier is oversized, including after extras merge
- **THEN** it is trimmed and capped before the upstream request

### Requirement: Copilot Claude response-header budget
GitHub Copilot Claude chat-completions requests SHALL use a dedicated configurable response-header timeout budget that is longer than the ordinary Copilot request budget, while model listing, embeddings, and non-Claude chat-completions SHALL retain the ordinary budget. Streaming response bodies SHALL remain unbounded after response headers arrive.

#### Scenario: Slow Claude response headers
- **WHEN** a Copilot Claude chat-completions request takes longer than the ordinary request budget but returns headers within the Claude budget
- **THEN** the request continues rather than failing at the ordinary timeout boundary

#### Scenario: Ordinary Copilot request
- **WHEN** a non-Claude chat-completions, model-list, or embeddings request is made
- **THEN** it uses the ordinary Copilot request timeout

#### Scenario: Claude budget exhausted
- **WHEN** a Copilot Claude request does not return headers within its configured budget
- **THEN** the attempt fails once and provider dispatch may proceed to its next candidate without an automatic duplicate request

### Requirement: Copilot timeout diagnostics
A Copilot upstream timeout SHALL emit one structured warning containing the provider instance, request identifier when available, canonical model, upstream endpoint, configured timeout budget, and elapsed duration, without including credentials or request content.

#### Scenario: Response-header timeout logged
- **WHEN** a Copilot request fails while awaiting response headers
- **THEN** the warning contains enough request, provider, endpoint, model, budget, and elapsed context to correlate the failure with the gateway request log

