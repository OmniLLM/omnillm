## 1. Specification and Regression Coverage

- [x] 1.1 Strictly validate the proposal, delta spec, design, and tasks, then
  obtain human approval before implementation.
- [x] 1.2 Add deterministic tests for provider and virtual-model choice labels,
  case-insensitive ID/name/alias search, legacy subtitle fallback, duplicate and
  absent metadata, invalid records, and safe control-character rendering.
- [x] 1.3 Add representative interactive provider show/enable and virtual-model
  command tests verifying that the chosen original ID reaches the server; cover
  cancellation, no choices, one choice, and non-interactive compatibility.

## 2. Interactive Selection

- [x] 2.1 Audit interactive choice call sites for any additional identifier-only
  entity selectors; retain existing behavior for non-entity prompts.
- [x] 2.2 Implement structured, searchable entity choices with safe display labels
  and original-ID selection for provider and virtual-model resolvers.
- [x] 2.3 Preserve explicit arguments, existing JSON and non-interactive behavior,
  and zero-item, one-item, and cancelled selection behavior.

## 3. Verification

- [x] 3.1 Run focused command tests, including under the race detector, and
  inspect the rendered interactive choices and selected IDs.
- [x] 3.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`,
  `bun test`, and `bun run build`.
- [x] 3.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 3.4 Confirm every task is complete and verified and the archive is the only
  operation updating current-state specs; archive the validated change afterward.
