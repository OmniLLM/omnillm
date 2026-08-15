## ADDED Requirements

### Requirement: Copilot advertised-model execution contract
GitHub Copilot models SHALL be discovered from the authenticated Copilot catalog and SHALL execute through a supported upstream API shape advertised for that model while preserving compatible generation, streaming, system-instruction, and tool-use semantics.

#### Scenario: Grok catalog discovery
- **WHEN** the authenticated Copilot catalog advertises a Grok model with capabilities and supported endpoints
- **THEN** OmniLLM exposes that exact model identifier and capability metadata and selects one of its advertised upstream API shapes

#### Scenario: Grok request through supported gateway shapes
- **WHEN** Chat Completions, Messages, or Responses input routes to an advertised Copilot Grok model
- **THEN** the Copilot adapter preserves the request's system instructions, conversation and tool history, and returns the requested gateway envelope with a valid terminal result

#### Scenario: Provider-pinned Grok request
- **WHEN** a Copilot Grok request is pinned through a provider-qualified or virtual-model route
- **THEN** endpoint selection remains consistent with the authenticated model catalog rather than falling back solely from the model name

#### Scenario: Grok stream cancellation
- **WHEN** the caller cancels a streaming Copilot Grok request
- **THEN** the in-flight upstream request is cancelled without a second upstream execution or duplicate downstream output
