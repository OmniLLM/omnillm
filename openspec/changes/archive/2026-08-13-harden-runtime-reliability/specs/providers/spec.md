## ADDED Requirements

### Requirement: Concurrent Copilot shape metadata
GitHub Copilot model-shape metadata SHALL support concurrent model-list refresh and request-shape lookup without data races, map mutation hazards, or partially published metadata.

#### Scenario: Model discovery overlaps request routing
- **WHEN** a successful Copilot model-list refresh publishes shape metadata while requests concurrently select an upstream API shape
- **THEN** each request observes either the previous complete snapshot or the new complete snapshot and shape selection remains valid

#### Scenario: Shape metadata is unavailable
- **WHEN** no complete shape snapshot has been published for a model
- **THEN** the existing model-family fallback determines the request shape
