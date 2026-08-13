## ADDED Requirements

### Requirement: Native Responses custom-tool representation
CIF SHALL preserve the native Responses custom-tool discriminator, custom definition format, raw call input, optional namespace, and original output value while retaining function-compatible argument and text fallbacks. Existing CIF values that omit the discriminator SHALL continue to represent function tools.

#### Scenario: Custom tool definition
- **WHEN** a Responses request declares a custom tool with a format
- **THEN** CIF preserves its custom kind and format while retaining a fallback schema with one required string `input`

#### Scenario: Raw custom call input
- **WHEN** a custom call contains arbitrary text, including an explicitly empty string
- **THEN** CIF preserves the exact raw input and custom kind without JSON parsing while retaining the same text under fallback argument `input`

#### Scenario: Original custom output
- **WHEN** a custom output is a string or supported ordered content list
- **THEN** CIF preserves the original value and custom kind while retaining normalized text content for non-Responses fallbacks

#### Scenario: Legacy function tool
- **WHEN** an existing CIF tool, call, or result omits the new kind field
- **THEN** all serializers and providers continue treating it as an ordinary function tool

### Requirement: Responses additional-tool name fidelity
Responses ingestion SHALL preserve the declared name of each nested `additional_tools` definition as the canonical callable tool name. Transport namespace labels SHALL NOT be prepended to the declared name unless a reversible client-facing mapping preserves the original name through the complete call round trip.

#### Scenario: Codex functions namespace
- **WHEN** Codex declares a nested custom tool named `exec` under an `additional_tools` namespace named `functions`
- **THEN** canonical translation exposes the callable name as `exec` and not `functions__exec`

#### Scenario: Duplicate declared names
- **WHEN** multiple nested namespace groups declare the same callable tool name
- **THEN** translation retains one deterministic canonical definition for that name rather than inventing namespace-prefixed names the client did not register
