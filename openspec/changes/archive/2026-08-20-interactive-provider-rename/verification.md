# Verification

## Behavior Coverage

- Interactive rename prompts for both display name and alias, trims the input,
  and sends the non-empty values in the existing PATCH request.
- Empty interactive values return local validation and send no request.
- Explicit name or alias values bypass prompts.
- Hidden `--subtitle` compatibility and alias/subtitle conflict validation are
  preserved.
- Non-interactive use without rename metadata returns immediately without a
  prompt.

## Commands

- `go test -race ./internal/commands -run 'Test(ResolveProviderRenameFields|RunProviderRename)'` — passed.
- `bun run spec:check` — passed; strict validation reported 15 passed and the
  mandatory OpenSpec gate passed.
- `go vet ./...` — passed.
- `go build ./...` — passed.
- `go test -race ./...` — passed.
- `git diff --check` — passed.
