## Context

See `proposal.md` for motivation. OpenAI currently owns PKCE, base64url state generation, authorization URL construction, JSON token requests, a fixed loopback callback listener, and OpenAI claims processing. Antigravity separately owns hexadecimal state generation, authorization URL construction, form-encoded token requests, a public gateway callback, state expiry, profile discovery, and persistence.

The duplicated protocol primitives are small, but they sit on a security-sensitive path. The design must improve reuse without merging materially different provider flows or changing existing internal provider APIs, external route shapes, redirect registrations, or persisted credentials.

## Goals / Non-Goals

**Goals:**

- Establish a narrow, standard-library-only internal package for provider-neutral authorization-code primitives.
- Centralize secure state generation, S256 PKCE generation, deterministic authorization URL assembly, and JSON token-response decoding.
- Keep existing provider function signatures as compatibility boundaries.
- Preserve provider-specific wire contracts and callback/state lifecycles.
- Make secure-random failure explicit and fail closed for both flows.

**Non-Goals:**

- Create a generic OAuth flow orchestrator, token client, callback server, state store, or provider interface.
- Change redirect URI derivation, callback routes, scopes, authorization parameters, client registration, or PKCE policy.
- Add PKCE to Antigravity or change OpenAI claims processing.
- Consolidate provider persistence, refresh scheduling, user-profile discovery, or OAuth status polling.
- Add external dependencies or expose a public Go module API.

## Decisions

### 1. Add a focused `internal/oauthcode` package

The package will contain only protocol-level primitives:

- a token response data type and JSON decoder;
- secure state generation with explicit base64url and lowercase-hex encodings;
- S256 PKCE verifier/challenge generation;
- authorization URL assembly from a caller-owned endpoint and `url.Values`.

It will use only Go's standard library and will not import routes, providers, persistence, Gin, or logging. This keeps dependency direction from providers and routes toward a small leaf package.

**Alternative considered:** place helpers in `internal/providers/shared`. Rejected because OAuth cryptography and protocol parsing are a distinct concern from shared provider HTTP transport and request utilities.

### 2. Share primitives, not complete flows

OpenAI and Antigravity will continue to own authorization parameters and token request construction. OpenAI remains JSON encoded and uses S256 PKCE with a temporary loopback listener. Antigravity remains form encoded, uses a client secret, and returns through a public gateway route.

Callback handling, pending-state maps, result polling, claims parsing, profile discovery, credential persistence, and refresh policy remain outside the library.

**Alternative considered:** a configurable OAuth client or flow manager. Rejected because its options would encode provider differences as conditionals, obscure security-sensitive behavior, and increase the chance of applying an unsafe default.

### 3. Preserve provider package APIs through aliases and wrappers

OpenAI will retain its current exported `PKCE`, `TokenResponse`, `GeneratePKCE`, `RandomState`, and `BuildAuthURL` surface, delegating their provider-neutral work to the shared package. Antigravity will retain its current token response type name and OAuth function signatures.

This avoids changes to route, provider, and command callers while permitting shared tests at the protocol boundary.

**Alternative considered:** migrate all callers directly to the new package. Rejected because it would leak protocol primitives into orchestration code and enlarge the refactor without improving behavior.

### 4. Preserve state formats while sharing secure generation

The generator will accept a constrained encoding choice rather than an arbitrary encoder:

- OpenAI state: 16 random bytes encoded with unpadded base64url.
- Antigravity state: 16 random bytes encoded as lowercase hexadecimal.
- OpenAI PKCE verifier: 32 random bytes encoded with unpadded base64url; challenge is the unpadded base64url SHA-256 digest of the encoded verifier.

Invalid byte lengths or unsupported encodings will return errors. Secure-random errors will propagate to the initiating route, which will return an internal error before storing pending state or returning an authorization URL.

**Alternative considered:** normalize both providers to one state encoding. Rejected because preserving provider compatibility and minimizing change is more important than making opaque state strings visually uniform.

### 5. Keep token transport and validation provider-owned

The shared decoder will decode the common JSON fields and return decoding errors without embedding the raw response body. Provider wrappers will continue to decide request encoding, headers, HTTP status diagnostics, OAuth error formatting, required fields, refresh-token behavior, and provider-specific claims processing.

This prevents the shared package from learning client secrets, transport policy, or provider error semantics.

**Alternative considered:** a shared token endpoint client with configurable encodings. Rejected because OpenAI and Antigravity differ in body format, headers, request fields, refresh parameters, and error handling; a common transport would be mostly branching configuration.

### 6. Pin behavior with layered regression tests

Shared-package tests will cover cryptographic and encoding properties, URL assembly, and token decoding. Provider tests will pin complete authorization parameters and token/refresh wire formats. Route and callback tests will pin state validation, expiry, consumption, random-failure handling, redirect URI reuse, and existing callback behavior.

Tests that bind OpenAI's fixed callback port will remain serialized.

## Risks / Trade-offs

- **[Risk] A broad abstraction could hide provider-specific security requirements** → Keep only leaf protocol primitives shared and retain request/callback orchestration in provider and route packages.
- **[Risk] Refactoring changes redirect URI or request serialization** → Add exact query/body regression tests and retain provider-owned construction with existing signatures.
- **[Risk] Type aliases expose fields a provider does not use** → Treat the shared response as a wire DTO only; provider wrappers continue enforcing their own invariants.
- **[Risk] Random-source failure is difficult to test deterministically** → Use a narrow package-level random reader seam in the shared package tests and a route-level generation seam only where needed to verify fail-closed behavior.
- **[Trade-off] Callback and state-store duplication remains** → Accept this duplication because lifecycle and trust-boundary differences make unification riskier than the maintenance cost.

## Migration Plan

1. Add the shared package and its unit tests without changing callers.
2. Convert OpenAI's protocol primitives to compatibility aliases and wrappers, then run OpenAI and callback regressions.
3. Convert Antigravity URL assembly and token decoding while preserving form requests, then run provider regressions.
4. Route Antigravity state generation through the shared secure generator and add fail-closed route tests.
5. Run focused tests followed by repository-wide OpenSpec, Go, Bun, lint, typecheck, and build checks.

The change is an in-place internal refactor with no data migration or staged deployment requirement. Rollback consists of reverting the provider delegation and removing the unused shared package; no stored data or client configuration is transformed.
