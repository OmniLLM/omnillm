## Why

OpenAI and Antigravity independently implement the same OAuth authorization-code primitives, including secure state generation, authorization URL encoding, and token-response decoding. Extracting those protocol-level operations into a focused internal library will reduce duplication and make their security properties testable in one place without hiding provider-specific OAuth behavior.

## What Changes

- Add a provider-neutral internal OAuth authorization-code library for cryptographically secure state generation, S256 PKCE generation, authorization URL assembly, and token-response decoding.
- Retain thin compatibility wrappers in the OpenAI and Antigravity provider packages so current internal APIs and callers remain stable.
- Preserve each provider's redirect URI, authorization parameters, token request encoding, refresh behavior, callback model, state lifecycle, claims handling, and persistence behavior.
- Make Antigravity authorization initiation fail closed if cryptographically secure state generation fails instead of continuing with unusable state.
- Add focused shared-library, provider, and route regression tests for the extracted behavior and provider-specific compatibility contracts.
- No public API, CLI response shape, persisted credential format, route path, or normal successful runtime behavior changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providers`: Define the provider-specific OAuth authorization-code compatibility behavior that the shared primitives must preserve.
- `security`: Require OAuth state and PKCE material to use cryptographically secure randomness and require flow initiation to fail closed when secure generation fails.

## Impact

- Adds `internal/oauthcode` as a standard-library-only internal package.
- Refactors `internal/providers/openai/oauth.go` and `internal/providers/antigravity/oauth.go` to delegate only provider-neutral primitives.
- Updates Antigravity OAuth state creation and start-route error handling in `internal/routes/admin_antigravity_oauth.go`.
- Adds and updates Go tests under the shared OAuth package, both provider packages, and OAuth route handlers.
- Does not add dependencies or change external endpoints, callbacks, storage schemas, frontend behavior, or deployment configuration.
