## 1. Specification and Tests

- [x] 1.1 Run `bun run spec:validate` and obtain human approval of the proposal,
  delta specification, design, and tasks before implementation.
- [x] 1.2 Add deterministic CLI tests for provider reference precedence, alias and
  name matching, ambiguity and missing providers, table/JSON output, and
  exclusion of synthetic secrets from configuration and unknown fields.
- [x] 1.3 Add deterministic CLI tests for model detail fields, exact matching
  including slash-containing IDs, missing models, server errors, table/JSON
  output, argument validation, help, and the legacy root model shim.

## 2. Implementation

- [x] 2.1 Implement and register safe `provider show`, including completion and
  reference resolution consistent with the documented provider rules.
- [x] 2.2 Implement and register `provider model show` using the existing model
  list endpoint and preserve legacy model command dispatch.
- [x] 2.3 Document both commands, JSON selection, and omitted provider config.

## 3. Verification

- [x] 3.1 Run focused CLI tests and verify command help and error behavior.
- [x] 3.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`,
  `bun test`, and `bun run build`.
- [x] 3.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 3.4 Record verification results and confirm all tasks are complete before
  archiving the validated change through OpenSpec.
