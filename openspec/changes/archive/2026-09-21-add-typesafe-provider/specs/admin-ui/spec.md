## ADDED Requirements

### Requirement: TypeSafe provider setup and capability visibility
The Providers workspace SHALL offer TypeSafe API-key setup, ordinary lifecycle
controls, and discovered model visibility. English and Chinese interface text
SHALL identify Jev as supporting typed evaluations through System One. The Chat
workspace SHALL exclude evaluation-only models from generation model selection.

#### Scenario: TypeSafe setup form
- **WHEN** an operator selects TypeSafe when adding a provider
- **THEN** the console presents a masked API-key field and explains its typed-evaluation capability in the selected language

#### Scenario: Evaluation model in console
- **WHEN** a TypeSafe model has been discovered
- **THEN** it appears in provider model management with evaluation-only capabilities and is excluded from Chat model selection
