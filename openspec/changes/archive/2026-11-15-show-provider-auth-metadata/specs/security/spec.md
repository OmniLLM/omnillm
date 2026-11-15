## ADDED Requirements

### Requirement: Safe provider inspection metadata
Provider inspection metadata returned by the administration API or displayed by the CLI MUST be selected using an explicit allowlist and MUST NOT expose client secrets, API keys, access or refresh tokens, authorization codes, PKCE material, OAuth state, session-specific authorization URLs, or unknown configuration fields. Displayed operator-configured endpoint information SHALL exclude userinfo, query strings, fragments, and arbitrary paths that could carry credentials. Metadata inspection MUST NOT expose credential data in error responses or logs.

#### Scenario: Secret-bearing source records
- **WHEN** provider configuration or credential records contain secret-bearing and unknown fields alongside a public client ID
- **THEN** only explicitly allowed non-secret metadata is returned and no secret-bearing or unknown field appears in CLI table output, JSON output, API responses, errors, or logs

#### Scenario: Endpoint contains embedded credentials
- **WHEN** a configured endpoint contains credentials in userinfo, query, fragment, or path
- **THEN** inspection does not display those components

#### Scenario: Active OAuth flow
- **WHEN** a provider has an active OAuth flow with a session-specific authorization URL
- **THEN** provider inspection returns only the stable endpoint and never the flow URL, state, authorization code, or PKCE values
