## MODIFIED Requirements

### Requirement: Provider and model detail inspection
The CLI SHALL provide `omnillm provider show <reference>` and `omnillm provider model show <reference> <model-id>` to inspect one configured provider or one of its catalog models. Provider references SHALL use exact instance ID, then unique case-insensitive alias, then unique case-insensitive display name, consistent with provider reference resolution. Model IDs SHALL match exactly, including embedded slashes. Both commands SHALL support table and JSON output. Provider details SHALL include the available ID, type, name, alias, authentication status, activation status, and model counts, together with available allowlisted non-secret authentication and provider metadata, such as a stable authorization endpoint and OAuth client ID. Model details SHALL include available ID, name, enablement, description, token limits, and capabilities. The provider show command MUST omit raw configuration, unknown fields, credentials, and temporary authorization material from both output formats.

#### Scenario: Show provider by alias
- **WHEN** an operator invokes `omnillm provider show` with a unique alias
- **THEN** the CLI displays the matching provider's safe identity and status fields

#### Scenario: Reference precedence
- **WHEN** an exact instance ID and another provider's alias or name match the supplied reference
- **THEN** the exact instance is shown

#### Scenario: Ambiguous provider reference
- **WHEN** the highest-precedence matching alias or display name belongs to multiple providers
- **THEN** the command fails without choosing a provider and identifies the matching instance IDs

#### Scenario: Show a namespaced model
- **WHEN** an operator invokes `omnillm provider model show` with a provider reference and an exact model ID containing slashes
- **THEN** the CLI displays that model's available catalog details without interpreting the slash as a provider boundary

#### Scenario: Show JSON
- **WHEN** an operator selects `--output json` for either show command
- **THEN** the CLI prints one selected detail object rather than the list response; provider output contains only the approved safe fields

#### Scenario: Missing provider or model
- **WHEN** the requested provider or exact model ID does not exist
- **THEN** the command returns a non-success result and an actionable not-found error

#### Scenario: Provider configuration contains secrets
- **WHEN** a provider list entry contains credential material in configuration or unknown fields
- **THEN** neither table nor JSON provider show output contains those fields or their values

#### Scenario: Existing model command compatibility
- **WHEN** an operator invokes the hidden legacy root form `omnillm model show <reference> <model-id>`
- **THEN** it performs the same inspection as `omnillm provider model show`

#### Scenario: Authentication metadata is available
- **WHEN** an operator shows a provider for which a stable OAuth authorization endpoint and public client ID are available
- **THEN** the CLI displays those fields in table and JSON output without displaying a temporary authorization URL

#### Scenario: Metadata unavailable on an older server
- **WHEN** provider inspection succeeds against an older server that does not expose the metadata endpoint
- **THEN** the CLI retains the existing safe details and reports that additional metadata is unavailable without inventing fields
