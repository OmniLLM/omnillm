## 1. Specification and tests

- [x] 1.1 Strictly validate the change artifacts and obtain human approval before implementation.
- [x] 1.2 Add deterministic tests for provider help, canonical and legacy names, API paths, reference resolution, completion, and JSON output.

## 2. CLI changes

- [x] 2.1 Implement the canonical command names and compatibility aliases without changing activation semantics.
- [x] 2.2 Update CLI diagnostics, success messages, and current documentation to use canonical names.

## 3. Verification

- [x] 3.1 Run `bun run spec:check` and the relevant Go CLI tests.
- [x] 3.2 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`; report any environmental limitations.

## 4. Remove legacy aliases

- [x] 4.1 Strictly validate the revised change and obtain human approval before implementation.
- [x] 4.2 Update deterministic tests to assert that the old names are rejected without sending requests while canonical commands still work.
- [x] 4.3 Remove the legacy Cobra aliases without changing the existing server API endpoints.
- [x] 4.4 Run `bun run spec:check`, the relevant Go CLI tests, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
