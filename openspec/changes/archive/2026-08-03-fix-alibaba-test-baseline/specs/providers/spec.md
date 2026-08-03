## MODIFIED Requirements

### Requirement: Provider compatibility transforms
Provider adapters SHALL preserve gateway contracts while applying verified provider-specific execution, discovery, and serialization rules.

#### Scenario: Alibaba client streaming
- **WHEN** a streaming request routes to Alibaba
- **THEN** the upstream call executes non-streaming and the canonical result is re-streamed locally

#### Scenario: Alibaba Qwen 3.6 thinking flag
- **WHEN** an Alibaba Qwen 3.6 Plus request is constructed for plain chat, tool use, or a locally re-streamed response
- **THEN** its upstream thinking flag matches the supported Alibaba contract consistently across provider and server execution paths

#### Scenario: Alibaba live model metadata
- **WHEN** Alibaba live model discovery returns a known model identifier
- **THEN** provider model output applies the available metadata-enriched display name consistently

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
