## ADDED Requirements

### Requirement: Anthropic legacy tuple schema compatibility
Anthropic tool ingestion SHALL normalize legacy array-valued `items` schemas into equivalent draft 2020-12 `prefixItems` schemas before provider dispatch. For a converted tuple, an explicitly supplied boolean or schema-valued `additionalItems` SHALL become `items`; absent `additionalItems` SHALL retain the legacy policy of permitting trailing items without imposing an extra constraint. Normalization MUST preserve positional schema order, nested constraints, tool identity, and existing modern tuple schemas, and MUST NOT mutate the supplied schema or silently remove tools.

#### Scenario: RGB tuple from an MCP tool
- **WHEN** an Anthropic request declares a tool with a draft-07 array-valued `items` RGB tuple
- **THEN** its canonical tool schema uses ordered `prefixItems` with the same integer bounds and retains the tool name, description, required properties, order, and cache control

#### Scenario: Closed legacy tuple
- **WHEN** a legacy tuple supplies `additionalItems: false`
- **THEN** the canonical tuple contains `items: false` and continues to reject extra elements

#### Scenario: Constrained trailing items
- **WHEN** a legacy tuple supplies a schema-valued `additionalItems`
- **THEN** that schema becomes `items` and continues to constrain all trailing elements

#### Scenario: Unconstrained trailing items
- **WHEN** a legacy tuple omits `additionalItems` or supplies `additionalItems: true`
- **THEN** normalization continues permitting trailing elements without introducing a maximum length

#### Scenario: Nested tuple schemas
- **WHEN** a legacy tuple occurs within supported schema-bearing object properties, definitions, array schemas, or schema combinators
- **THEN** normalization converts that tuple while preserving its location and constraints

#### Scenario: Literal values and schema property names
- **WHEN** a tool schema contains examples, defaults, constants, enums, or property and definition names resembling schema keywords
- **THEN** tuple conversion leaves those literal values and names unchanged and traverses only actual schema values

#### Scenario: Modern tuple and ordinary array
- **WHEN** a tool schema already uses `prefixItems` or uses ordinary schema-valued `items`
- **THEN** legacy tuple conversion leaves its structure and accepted values unchanged

#### Scenario: Source schema reuse
- **WHEN** a source tool schema is reused after normalization
- **THEN** its original tuple syntax and nested values remain unchanged
