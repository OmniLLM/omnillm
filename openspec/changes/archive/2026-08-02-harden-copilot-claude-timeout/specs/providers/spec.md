## ADDED Requirements

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
