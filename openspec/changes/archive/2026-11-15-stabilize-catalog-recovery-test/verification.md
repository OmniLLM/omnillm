## Diagnosis

- PR CI failed in `TestCatalogRefreshRestoresAllClientToolHistories` because fixture re-registration returned `provider is busy` for all client subtests.
- The failure reproduced locally in ten repeated race-test runs before the change. Activation schedules background persistence, which borrows the provider and can overlap fixture replacement.
- The test now waits for pending saves before replacing the fixture. Provider lease semantics and client-history assertions remain unchanged.

## Verification

- `go test -race ./internal/server -run '^TestCatalogRefreshRestoresAllClientToolHistories$' -count=100` passed.
- `bun run spec:check`, `go vet ./...`, `go build ./...`, and `go test -race ./...` passed.
- No production runtime code was changed. The repeated test establishes observed stability, not proof that all possible scheduling interleavings have been exercised.
