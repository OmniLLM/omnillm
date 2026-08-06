## 1. Contract and Regression Tests

- [x] 1.1 Add shared routing unit tests for complete native identifier precedence, recognized instance and subtitle fallback qualifiers, unknown namespace segments, and explicitly qualified native slash-containing identifiers.
- [x] 1.2 Update unknown-prefix regression expectations so an unrecognized first segment remains part of the complete model identifier and fails only through ordinary model resolution when unavailable.
- [x] 1.3 Add Alibaba adapter tests proving `kimi/kimi-k3` and its whitespace-normalized form retain all namespace segments.
- [x] 1.4 Add cross-provider integration tests proving an advertised complete namespaced model wins even when its namespace collides with a provider prefix, while explicit qualification still preserves full identifiers, priority, and failover behavior.
- [x] 1.5 Add chat completions, Anthropic Messages, and Responses parity tests for provider pinning and native slash-containing model identifiers.

## 2. Shared Model Resolution

- [x] 2.1 Build the complete native model attempt before any recognized instance-identifier or subtitle qualification fallback.
- [x] 2.2 Preserve ordinary provider ordering and failover when the complete identifier has candidates, including provider-prefix collisions.
- [x] 2.3 Add a pinned fallback that removes only one recognized provider qualifier and preserves the complete remaining namespaced model identifier when the full attempt has no candidates.

## 3. Provider and Dialect Dispatch

- [x] 3.1 Change Alibaba model remapping to preserve internal slash namespaces while retaining required normalization.
- [x] 3.2 Audit provider remappers for duplicate provider-qualifier stripping and correct any destructive shared-routing inference covered by the delta specs.
- [x] 3.3 Refactor Responses generation dispatch to retain resolved attempts, including provider identity and ordered failover candidates, consistently with chat completions and Messages.

## 4. Verification and Completion

- [x] 4.1 Run `bun run spec:check` and confirm the implemented behavior still conforms to every revised delta scenario.
- [x] 4.2 Run focused Go tests for model routing, Alibaba, provider dispatch, generation route integration, and the live-discovered provider-prefix collision.
- [x] 4.3 Run `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 4.4 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 4.5 Review the final diff, repeat the temporary-port live test for `kimi/kimi-k3`, check each task only after verification completes, and archive only after all checks pass.
