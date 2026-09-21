## ADDED Requirements

### Requirement: TypeSafe CLI authentication
The CLI SHALL offer `typesafe` in provider-add, auth, help, completion, and
interactive provider selection. A non-empty explicit `--api-key` SHALL take
precedence over `TYPESAFE_API_KEY` from the CLI process environment. When neither
is available, interactive setup SHALL prompt securely and non-interactive setup
SHALL fail with an actionable missing-key error. The CLI MUST NOT automatically
source shell files or print the key.

#### Scenario: Environment key
- **WHEN** the operator sources their credential file, exports TYPESAFE_API_KEY, and invokes `omnillm provider add typesafe`
- **THEN** the CLI uses the environment key through the existing authenticated provider setup API

#### Scenario: Explicit key overrides environment
- **WHEN** an explicit non-empty API-key flag and environment key are both supplied
- **THEN** the explicit value is used

#### Scenario: Missing key without terminal
- **WHEN** non-interactive TypeSafe setup has no explicit or environment key
- **THEN** the CLI exits with a missing-key error without creating a provider
