## 1. Specification and approval

- [x] 1.1 Run `bun run spec:validate` and obtain human approval of the revised proposal, delta specs, design, and tasks before continuing implementation.

## 2. Regression tests

- [x] 2.1 Add deterministic tests for UUID format, collision retry, concurrent creation, the first-start default provider, every immediate and interactive provider creation path, OAuth completion, failed creation cleanup, re-authentication, refresh, and legacy ID preservation.
- [x] 2.2 Add SQLite tests for transferring every owned record and exact virtual-model provider reference, per-upstream hint creation, repeated renames and cleanup, preserving unrelated data and historical logs, transaction rollback, populated database upgrade, and restart.
- [x] 2.3 Add authenticated API and lifecycle tests for ID validation, collisions with IDs/aliases/names, no-op, authorization, busy providers and OAuth flows, active and primary selection, stale background snapshots, lease acquisition and release on cancellation, concurrent operations, and cache invalidation.
- [x] 2.4 Add routing tests for renamed active providers, multi-provider and streaming leases, virtual-model upstreams including stored old-ID qualifiers across restart and repeated renames, model enablement, preserved native model identifiers, and obsolete cache isolation.
- [x] 2.5 Add frontend tests for accessible edit, confirmation and warning, cancel, validation and server errors, authoritative reload, and existing name/alias controls.

## 3. Implementation

- [x] 3.1 Implement unique UUID allocation and reservation across first-start default provider creation, provider creation and interactive auth flows while retaining legacy re-authentication and refresh identities.
- [x] 3.2 Implement atomic durable identity transfer with validated targets, correct foreign-key handling, rollback, preserved metadata, exact internal virtual-upstream references, and durable, scoped per-upstream qualifier hints with cleanup.
- [x] 3.3 Implement and audit per-provider leases across routing, provider-scoped work, interactive authentication, credential refresh, and background persistence; prepare the replacement provider before commit, publish it with active and primary selection after commit, and invalidate routing caches safely.
- [x] 3.4 Expose the authenticated provider ID update operation, preserving existing metadata and login API contracts.
- [x] 3.5 Add the Providers workspace edit control, warning, error handling, and authoritative refresh, with any necessary localized strings.
- [x] 3.6 Update relevant operator/API documentation and in-repository clients to use returned IDs and explain external-reference migration.

## 4. Verification and archive

- [x] 4.1 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 4.2 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`, including the new lifecycle and routing regression tests.
- [x] 4.3 Record outcomes and resolve all failures; archive the validated change only after every preceding task is complete and verified.
