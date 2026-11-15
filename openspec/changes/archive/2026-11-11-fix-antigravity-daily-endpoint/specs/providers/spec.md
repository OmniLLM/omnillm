## ADDED Requirements
### Requirement: Antigravity daily generation endpoint
Antigravity SHALL default generation to https://daily-cloudcode-pa.googleapis.com and SHALL attempt project discovery there before the existing fallback endpoints. Explicitly supplied generation base URLs MUST remain usable. Endpoint selection MUST NOT rewrite provider identity, stored credentials, or an existing project ID.

#### Scenario: Default generation
- **WHEN** an Antigravity instance generates without an explicit endpoint override
- **THEN** streaming and collected generation use the daily endpoint

#### Scenario: Explicit endpoint
- **WHEN** a generation call supplies a base URL
- **THEN** execution uses that URL rather than substituting the daily default

#### Scenario: Project discovery
- **WHEN** OAuth project discovery runs
- **THEN** it attempts the daily endpoint first and retains the existing fallback behavior if discovery fails

### Requirement: Gemini 3.8 Flash effort catalog
Antigravity SHALL advertise gemini-3.8-flash-low, gemini-3.8-flash-medium, and gemini-3.8-flash-high with their corresponding effort labels and SHALL preserve those complete identifiers in generation requests.

#### Scenario: Model listing
- **WHEN** an Antigravity model catalog is freshly retrieved
- **THEN** all three Gemini 3.8 Flash effort variants are available alongside the existing catalog entries

#### Scenario: Explicit effort selection
- **WHEN** a request selects gemini-3.8-flash-low
- **THEN** the upstream model remains gemini-3.8-flash-low without silently switching effort tiers
