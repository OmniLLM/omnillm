# Recover OpenAI refresh-token rotation failures

## Why

OpenAI ChatGPT OAuth refresh tokens are rotated and single-use. When another
provider object or process consumes a persisted refresh token first, OmniLLM
retries the rejected token on every request. OpenAI now returns this condition
as a nested JSON error object, which the current decoder misclassifies as a
token-response parse failure and includes as a multiline raw-body preview in
operator logs.

## What changes

- Decode both standard string OAuth errors and OpenAI's nested error envelope
  into a stable, sanitized token error.
- When refresh-token rotation is rejected, reload the provider's durable token
  record and retry once only if it contains a different refresh token.
- When no newer durable token exists, discard the rejected refresh token,
  persist that state, and return actionable reauthentication guidance without
  retrying it on subsequent requests.
- Add deterministic tests for nested error decoding, rotated-token recovery,
  terminal reauthentication state, persistence, and concurrent refreshes.

## Impact

This changes runtime behavior for the `openai` ChatGPT-OAuth provider and its
provider-owned credential persistence. It does not change public HTTP API
shapes, database schema, or other providers.

## Capabilities

- `providers`: OpenAI OAuth error handling and refresh-token rotation recovery.
- `persistence`: durable rejection of an unusable provider refresh token.
