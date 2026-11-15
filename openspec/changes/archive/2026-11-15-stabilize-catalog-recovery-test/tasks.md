## 1. Specification and diagnosis

- [x] 1.1 Strictly validate the change artifacts and obtain human approval before implementation.
- [x] 1.2 Confirm the timing failure and identify the lifecycle work that must finish before fixture replacement.

## 2. Stabilize the test

- [x] 2.1 Synchronize fixture setup with outstanding provider work without changing runtime lease behavior or removing client-history assertions.
- [x] 2.2 Verify the affected test repeatedly under the race detector, including all client fixtures.

## 3. Verification

- [x] 3.1 Run `bun run spec:check`, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 3.2 Record results and any limitations before archiving the validated change.
