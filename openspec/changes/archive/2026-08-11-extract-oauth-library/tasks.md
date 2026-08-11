## 1. Contract and Test Preparation

- [x] 1.1 Run `bun run spec:validate` and confirm the approved provider and security deltas remain strict-valid before implementation begins.
- [x] 1.2 Add shared OAuth primitive tests for state encodings, entropy-derived lengths, S256 PKCE properties, URL encoding, token decoding, malformed responses, and unsupported inputs.
- [x] 1.3 Add OpenAI regression tests that pin the complete authorization query, fixed redirect URI, JSON code/refresh requests, PKCE verifier use, OAuth errors, and refresh-token retention.
- [x] 1.4 Add Antigravity regression tests that pin the complete authorization query, caller-derived redirect URI, form-encoded code/refresh requests, OAuth errors, and refresh response behavior.
- [x] 1.5 Add Antigravity route tests for state format, secure-generation failure, missing/unknown/expired state rejection, consume-before-exchange behavior, and exact redirect URI reuse.

## 2. Shared OAuth Library

- [x] 2.1 Create the standard-library-only `internal/oauthcode` package with common token response fields and JSON decoding.
- [x] 2.2 Implement cryptographically secure state generation with constrained unpadded-base64url and lowercase-hex encodings and explicit error propagation.
- [x] 2.3 Implement 32-byte S256 PKCE verifier/challenge generation while preserving the current OpenAI encoding and hashing behavior.
- [x] 2.4 Implement deterministic authorization URL assembly from a caller-owned endpoint and query values.
- [x] 2.5 Run `go test ./internal/oauthcode` and fix all shared primitive test failures.

## 3. Provider Integration

- [x] 3.1 Refactor OpenAI OAuth types and helpers into compatibility aliases/wrappers over `internal/oauthcode` while retaining provider-specific parameters, JSON transport, errors, claims, and refresh behavior.
- [x] 3.2 Run `go test ./internal/providers/openai` and confirm the fixed callback, PKCE, token exchange, refresh, claims, and listener tests pass.
- [x] 3.3 Refactor Antigravity authorization URL assembly and token decoding over `internal/oauthcode` while retaining provider-specific scopes, form transport, client-secret handling, errors, profile discovery, and refresh behavior.
- [x] 3.4 Refactor Antigravity state creation to use shared secure generation, preserve 16-byte lowercase-hex output, and fail flow initiation before state storage or auth URL issuance on generation errors.
- [x] 3.5 Run `go test ./internal/providers/antigravity ./internal/routes ./internal/commands` and fix all OAuth compatibility failures.

## 4. Verification and Completion

- [x] 4.1 Run `gofmt` on changed Go files and confirm no unrelated files are modified.
- [x] 4.2 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 4.3 Run `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 4.4 Run `bun run spec:check` and confirm all OpenSpec and implementation checks pass.
- [x] 4.5 Review the final diff for provider compatibility, credential secrecy, redirect URI stability, and absence of scope creep; check each task only after its evidence passes.
- [x] 4.6 Archive with `bunx openspec archive extract-oauth-library --yes` only after every task is checked and all verification passes.
