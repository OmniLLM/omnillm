## MODIFIED Requirements

### Requirement: Provider type catalog
The system SHALL support distinct provider types for GitHub Copilot, Antigravity, Alibaba, ModelScope, Azure OpenAI, Google, Kimi, OpenAI-compatible endpoints, Codex API keys, OpenAI OAuth accounts, and TypeSafe.

#### Scenario: Distinct OpenAI modes
- **WHEN** provider types are listed
- **THEN** API-key Codex and OAuth OpenAI accounts remain distinct types

#### Scenario: TypeSafe provider type
- **WHEN** provider types are listed
- **THEN** TypeSafe is identified as `typesafe` with API-key authentication

## ADDED Requirements

### Requirement: TypeSafe credential lifecycle
The TypeSafe provider SHALL authenticate using a non-empty API key, validate it
through authenticated model discovery before successful setup, persist it per
instance using existing credential storage, and reconstruct that instance after
restart. The key MUST NOT appear in identifiers, public metadata, errors, or logs.

#### Scenario: Successful setup and restart
- **WHEN** a TypeSafe key successfully fetches the upstream catalog and the gateway restarts
- **THEN** the same instance remains available with its persisted key and metadata

#### Scenario: Failed authentication
- **WHEN** upstream model discovery rejects a newly supplied key
- **THEN** setup fails without retaining a partially created provider or replacing an existing instance's working credentials

### Requirement: TypeSafe native model discovery
TypeSafe discovery SHALL call `GET https://api.typesafe.ai/v1/models` using bearer
authentication and map the upstream `models[].name` and description into provider
models. Each model SHALL declare System One evaluation support and explicitly
exclude text generation, streaming, tools, vision, and embeddings. Discovery MUST
retain the existing cache, enablement, forced-refresh, and degradation contracts.

#### Scenario: Alias catalog
- **WHEN** discovery returns `jev-latest` and `jev-preview`
- **THEN** both aliases appear with their descriptions and evaluation-only capabilities

#### Scenario: Catalog failure
- **WHEN** discovery fails after a successful catalog has been cached
- **THEN** existing degraded display behavior may use prior data without recording the failure as fresh successful discovery

### Requirement: TypeSafe native execution
TypeSafe evaluation SHALL send model, state, and questions to
`https://api.typesafe.ai/v1/systemone`, preserving structured instructions,
criteria, question identifiers, and JSON values. It SHALL return upstream answers,
resolved model, and usage without converting probabilities to booleans, dropping
confidence or distributions, or inventing generated text. Execution MUST respect
caller cancellation and a finite request timeout.

#### Scenario: Mixed primitive batch
- **WHEN** a request includes Noul, Choice, and Score questions over the same state
- **THEN** one upstream evaluation receives all questions and all typed answers remain associated with their original identifiers

#### Scenario: Unsupported legacy operation
- **WHEN** a TypeSafe provider is invoked through a chat, streaming, or embedding execution method
- **THEN** it returns an explicit unsupported-operation error without calling an upstream generation endpoint

#### Scenario: Caller cancellation
- **WHEN** the caller cancels an in-flight evaluation
- **THEN** upstream work is canceled and no subsequent provider attempt starts
