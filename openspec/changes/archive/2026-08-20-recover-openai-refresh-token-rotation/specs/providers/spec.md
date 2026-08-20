## ADDED Requirements

### Requirement: OpenAI refresh-token rotation recovery

The OpenAI ChatGPT-OAuth provider SHALL decode standard and nested token
endpoint errors without logging raw credential-response bodies, SHALL recover a
rejected single-use refresh token by retrying at most once with a different
newer token from provider-owned durable state, and MUST stop automatically
retrying a rejected token when no newer durable token exists.

#### Scenario: Newer durable refresh token

- **WHEN** an OpenAI refresh token is rejected as already used and the durable
  provider record contains a different non-empty refresh token
- **THEN** the provider adopts that token, retries the exchange once, and
  persists the successful rotated token set

#### Scenario: No newer durable refresh token

- **WHEN** an OpenAI refresh token is rejected as already used and durable state
  contains no different refresh token
- **THEN** the provider retains its existing access token, durably retires the
  rejected refresh token, returns browser sign-in guidance, and makes no
  automatic refresh request with that rejected token later

#### Scenario: Nested token endpoint error

- **WHEN** OpenAI returns a nested JSON error object from the token endpoint
- **THEN** the provider returns a sanitized error containing actionable type,
  code, and message fields without including a raw response-body preview

#### Scenario: Concurrent refresh callers

- **WHEN** concurrent requests on one provider instance require refresh
- **THEN** they share the bounded refresh operation and do not independently
  consume the same single-use token
