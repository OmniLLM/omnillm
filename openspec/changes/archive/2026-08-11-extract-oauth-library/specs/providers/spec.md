## ADDED Requirements

### Requirement: OAuth authorization-code provider compatibility
OpenAI and Antigravity OAuth authorization-code flows SHALL preserve their provider-specific authorization parameters, redirect URI behavior, token request encoding, refresh semantics, callback handling, and token processing when using shared protocol primitives.

#### Scenario: OpenAI authorization initiation
- **WHEN** an OpenAI OAuth authorization flow is started
- **THEN** the authorization request uses the fixed loopback redirect URI, S256 PKCE, the existing OpenAI scopes, and the existing OpenAI-specific authorization parameters

#### Scenario: OpenAI code exchange
- **WHEN** a validated OpenAI callback code is exchanged
- **THEN** the token request uses JSON encoding, the same fixed redirect URI, and the matching PKCE verifier

#### Scenario: Antigravity authorization initiation
- **WHEN** an Antigravity OAuth authorization flow is started
- **THEN** the authorization request uses the caller-derived callback URI, the existing Google scopes, offline access, consent prompting, and the generated state

#### Scenario: Antigravity code exchange
- **WHEN** a validated Antigravity callback code is exchanged
- **THEN** the token request uses form encoding and the exact redirect URI stored when authorization began

#### Scenario: Provider refresh compatibility
- **WHEN** OpenAI or Antigravity refreshes an access token
- **THEN** each provider preserves its existing request parameters, response handling, and refresh-token retention behavior
