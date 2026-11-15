## MODIFIED Requirements

### Requirement: Layered deterministic compatibility matrix
The test suite SHALL verify model compatibility through separate client-shape-to-canonical, canonical-to-provider, and thin end-to-end matrix layers so that every supported API shape and provider execution strategy is covered without an unbounded Cartesian suite. The catalog recovery compatibility coverage SHALL run reliably without fixture lifecycle operations racing outstanding provider work.

#### Scenario: Supported inbound generation shapes
- **WHEN** deterministic compatibility tests run
- **THEN** OpenAI Chat Completions, Anthropic Messages, and OpenAI Responses shapes each execute plain, streaming, and tool-use semantic fixtures

#### Scenario: Supported provider strategies
- **WHEN** deterministic provider contract tests run
- **THEN** every supported provider type and each distinct upstream execution strategy has at least one representative compatible model row

#### Scenario: Dynamic upstream shape
- **WHEN** a provider can select Chat Completions or Responses according to model or configuration
- **THEN** the matrix includes representative rows for every supported upstream shape

#### Scenario: Repeated catalog recovery coverage
- **WHEN** catalog recovery tests exercise each supported client tool-history fixture repeatedly under the race detector
- **THEN** fixture setup completes without intermittent provider-busy errors and each recovery still verifies the expected tool history
