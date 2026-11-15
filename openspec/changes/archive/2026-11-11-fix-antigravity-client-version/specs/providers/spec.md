## ADDED Requirements
### Requirement: Consistent Antigravity generation client identity
Antigravity generation SHALL use one maintained client identity consistently in the HTTP User-Agent header and JSON userAgent field for both streaming and collected responses. Updating that identity MUST preserve the existing model, project, tools, message history, and authentication behavior.

#### Scenario: Generation request metadata
- **WHEN** an Antigravity generation request is constructed
- **THEN** the HTTP User-Agent and JSON userAgent both contain antigravity/2.6.0 rather than the retired antigravity/1.19.6 identity

#### Scenario: Streaming and collected requests
- **WHEN** either streaming or collected generation is selected
- **THEN** both paths use the same maintained identity and retain the original model, project, tools, and history
