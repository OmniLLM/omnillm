## ADDED Requirements

### Requirement: Consistent generation model resolution
The gateway SHALL apply the same provider-qualification, native model identifier preservation, provider pinning, provider ordering, and failover behavior to OpenAI-compatible chat completions, Anthropic Messages, and Responses generation requests.

#### Scenario: Native namespaced model across generation dialects
- **WHEN** equivalent generation requests specify a native slash-containing model identifier through chat completions, Anthropic Messages, or Responses
- **THEN** each dialect resolves and forwards the same complete native model identifier using ordinary provider selection

#### Scenario: Provider-qualified namespaced model across generation dialects
- **WHEN** equivalent generation requests specify a recognized provider qualifier followed by a slash-containing native model identifier
- **THEN** each dialect pins the same provider and forwards the same complete native model identifier after removing only the qualifier

#### Scenario: Selected provider fails
- **WHEN** an unqualified native namespaced model resolves to multiple providers and the leading provider returns a retryable failure
- **THEN** each generation dialect may continue through the same remaining provider candidates
