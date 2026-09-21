## 1. Specification and approval

- [x] 1.1 Inspect current provider, gateway, routing, CLI, administration, security, and compatibility contracts and the current TypeSafe API documentation.
- [x] 1.2 Confirm the supplied credential supports read-only model discovery without exposing its value.
- [x] 1.3 Prepare proposal, capability deltas, design, and ordered tasks.
- [x] 1.4 Run `bun run spec:validate` and resolve all strict validation errors (15 passed, 0 failed).
- [x] 1.5 Obtain human approval of the proposal, delta specs, design, and tasks before code, test, dependency, or runtime configuration changes (user: "proceed").

## 2. Deterministic tests before implementation

- [x] 2.1 Add fake-upstream provider tests for authentication, restart reconstruction, native model discovery, refresh failure, all three primitives, status handling, cancellation, timeout, and credential redaction.
- [x] 2.2 Add gateway and routing tests for native validation, authentication, concurrency, provider qualification, version pinning, enablement, virtual models, eligible failover, metering, and rejection across generation dialects/embeddings.
- [x] 2.3 Add admin, CLI, and frontend coverage for setup/rollback, environment precedence, model capabilities, localized setup, and Chat selection exclusion; extend the compatibility manifest with explicit unsupported-operation reasons.

## 3. Implementation

- [x] 3.1 Implement the native TypeSafe provider and evaluation interface, lifecycle factory registration, credential restoration, and catalog mapping without an SDK dependency.
- [x] 3.2 Implement `/v1/systemone`, schema validation, bounded native execution, sanitized errors, metering, and operation-aware candidate selection while preserving generation semantics.
- [x] 3.3 Add CLI authentication and environment fallback, admin lifecycle support, and localized TypeSafe setup and capability visibility in the console.
- [x] 3.4 Document credential loading, registration, native usage, model pinning, unsupported generation operations, and client-side backoff behavior.

## 4. Verification and archive

- [x] 4.1 Pass all targeted new regression tests and existing deterministic five-turn Claude Code, Codex CLI, Droid, and GitHub Copilot CLI client-shape checks.
- [x] 4.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`; resolve failures caused by this change and record any independently reproduced baseline failures.
- [x] 4.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`; resolve failures caused by this change and record any independently reproduced baseline failures.
- [x] 4.4 Run an isolated, fresh-port TypeSafe smoke using the supplied environment credential: authenticate, discover models, and evaluate one mixed-primitive request; record sanitized evidence.
- [x] 4.5 Run bounded five-call live coding-agent checks with compatible configured generation models where prerequisites exist; record concrete skips and TypeSafe's native-tool incompatibility without counting them as passes.
- [x] 4.6 Review the final diff for secrets and unintended changes, and record verification evidence.
- [x] 4.7 After approval, completed tasks, and passing verification, archive with `bunx openspec archive add-typesafe-provider --yes` and rerun `bun run spec:check`.
