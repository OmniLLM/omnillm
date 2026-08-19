## ADDED Requirements

### Requirement: Provider-qualified model aliases

Provider-qualified model names SHALL resolve a case-insensitive alias against
the persisted subtitle of a registered provider instance, while preserving
exact instance-ID resolution and the model identifier after the prefix.

#### Scenario: Resolve the configured GitHub Copilot alias

- **GIVEN** provider instance `github-copilot-jian-zhu---zhujian0805` is
  registered with subtitle `jzhu`
- **WHEN** a request uses model `jzhu/<model>`
- **THEN** OmniLLM routes the request to that provider instance with
  `<model>` as the upstream model identifier

#### Scenario: Preserve existing instance-ID resolution

- **GIVEN** provider instance `github-copilot-jian-zhu---zhujian0805` is
  registered
- **WHEN** a request uses the exact provider-qualified prefix
  `github-copilot-jian-zhu---zhujian0805/<model>`
- **THEN** OmniLLM routes the request to the same provider instance with
  `<model>` unchanged
