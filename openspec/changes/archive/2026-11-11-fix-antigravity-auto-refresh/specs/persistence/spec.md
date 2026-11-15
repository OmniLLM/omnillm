## ADDED Requirements
### Requirement: Durable Antigravity OAuth renewal
Antigravity OAuth completion and refresh SHALL persist absolute expiry when the token response supplies a positive lifetime. Refresh MUST preserve provider metadata and an existing refresh token when no replacement is returned. Credential updates MUST NOT overwrite a newer credential snapshot persisted concurrently.

#### Scenario: OAuth completion and restart
- **WHEN** OAuth supplies a token lifetime and the service restarts after saving credentials
- **THEN** the provider uses the persisted expiry to determine renewal needs

#### Scenario: Refresh token omitted
- **WHEN** a successful refresh returns only a new access token and expiry
- **THEN** the existing refresh token, OAuth client credentials, project, email, and unrelated metadata remain stored

#### Scenario: Rotated refresh token
- **WHEN** a successful refresh supplies a replacement refresh token
- **THEN** the new access token, refresh token, and expiry are persisted together

#### Scenario: Concurrent re-authentication
- **WHEN** OAuth completion replaces credentials while an older refresh is in flight
- **THEN** the older refresh cannot overwrite the newer credentials and execution reloads the current record

#### Scenario: Save failure
- **WHEN** persisting refreshed credentials fails
- **THEN** the provider reports failure without publishing the refreshed snapshot as successfully saved
