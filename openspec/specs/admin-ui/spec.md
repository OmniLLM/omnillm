# admin-ui Specification

## Purpose
Defines the browser administration console, its server delivery contract, navigation, persistent presentation preferences, and access to OmniLLM operational workspaces.
## Requirements
### Requirement: Administrative console delivery
The system SHALL serve the single-page administration console at `/admin/`, redirect `/admin` to that path, and serve built assets when they are available.

#### Scenario: Missing trailing slash
- **WHEN** a browser requests `/admin`
- **THEN** the server permanently redirects it to `/admin/`

#### Scenario: Missing built console
- **WHEN** the console index cannot be read
- **THEN** the server returns a minimal HTML document with a root mount point rather than failing startup

### Requirement: Browser authentication bootstrap
The server SHALL HTML-escape and inject its inbound API key into the console document so browser API requests can authenticate without a separate login flow.

#### Scenario: API-key placeholder exists
- **WHEN** the source document contains an empty `omnillm-api-key` meta tag
- **THEN** the server replaces it with the escaped configured key

### Requirement: Operational workspaces
The console SHALL provide Providers, Chat, Logging, Metering, Virtual Models, Config, Access Tokens, and About workspaces and render one selected workspace at a time.

#### Scenario: Workspace selection
- **WHEN** an operator selects a workspace
- **THEN** the console renders that workspace and updates the URL fragment

### Requirement: Workspace persistence
The console SHALL initialize its workspace from a recognized URL fragment, then the persisted selection, and otherwise Metering, and SHALL continue operating when local storage is unavailable.

#### Scenario: Deep link overrides persistence
- **WHEN** the URL fragment names a known workspace
- **THEN** that workspace opens regardless of the persisted last selection

#### Scenario: Storage unavailable
- **WHEN** reading or writing local storage throws
- **THEN** the console remains usable and falls back to Metering when no fragment selects a workspace

### Requirement: Persistent presentation preferences
The console SHALL support dark and light themes and a collapsible navigation sidebar, persisting both choices when storage is available.

#### Scenario: Default theme
- **WHEN** no valid theme preference can be read
- **THEN** the console applies the dark theme

### Requirement: Server information and localization
The console SHALL load public server information after mounting and SHALL render interface strings through a runtime-switchable translation layer.

#### Scenario: Language switch
- **WHEN** an operator changes the active language
- **THEN** visible translated strings re-render in the selected language

### Requirement: Metering prompt-cache visibility
The Metering workspace SHALL display provider prompt-cache status and cache token details for individual requests and SHALL display cache hit, miss, unknown, read-token, and write-token aggregates for the selected filters.

#### Scenario: Cache-hit request row
- **WHEN** a metering row has prompt-cache status `hit`
- **THEN** the Usage page displays a distinct hit indicator and its reported cache-read token count

#### Scenario: Unknown cache status
- **WHEN** a metering row has prompt-cache status `unknown`
- **THEN** the Usage page displays unknown rather than presenting the request as a cache miss

#### Scenario: Responsive cache columns
- **WHEN** cache details are shown on a narrow viewport or within a dense table
- **THEN** the operator can still inspect the status and token values without losing existing request identity and total usage fields

### Requirement: Prompt-cache terminology
The Metering workspace SHALL label provider prompt-cache status separately from OmniLLM exact-response-cache behavior and SHALL NOT describe an unknown provider counter as a miss.

#### Scenario: Provider prompt-cache legend
- **WHEN** an operator views cache metrics
- **THEN** labels or explanatory text identify them as provider prompt-cache usage and define hit, miss, and unknown consistently

