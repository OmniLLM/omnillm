## ADDED Requirements

### Requirement: Secure OAuth authorization material
OAuth authorization state and PKCE material SHALL be generated from a cryptographically secure random source with provider-compatible entropy and encoding, and the system SHALL NOT issue an authorization URL when secure generation fails.

#### Scenario: OpenAI authorization material
- **WHEN** an OpenAI OAuth flow starts successfully
- **THEN** it uses a 32-byte unpadded base64url PKCE verifier with an S256 challenge and a 16-byte unpadded base64url state value

#### Scenario: Antigravity authorization state
- **WHEN** an Antigravity OAuth flow starts successfully
- **THEN** it uses a 16-byte state value encoded as 32 lowercase hexadecimal characters

#### Scenario: Secure random generation fails
- **WHEN** secure random generation fails while an OAuth flow is starting
- **THEN** flow initiation fails without storing pending state or returning an authorization URL

### Requirement: OAuth callback state validation
An OAuth authorization code SHALL be exchanged only after the callback state is present, valid for the pending flow, unexpired where expiry is tracked, and consumed according to the provider flow's existing lifecycle.

#### Scenario: Missing or mismatched state
- **WHEN** an OAuth callback omits state or presents a value that does not match a pending flow
- **THEN** the callback is rejected and its authorization code is not exchanged

#### Scenario: Expired Antigravity state
- **WHEN** an Antigravity callback presents an expired pending state
- **THEN** the callback is rejected and its authorization code is not exchanged

#### Scenario: Valid Antigravity state
- **WHEN** an Antigravity callback presents a valid pending state
- **THEN** the state is consumed before the authorization code exchange begins
