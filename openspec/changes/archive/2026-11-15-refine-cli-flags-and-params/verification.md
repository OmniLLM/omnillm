## Verification

- `bun run spec:check` passed (`openspec validate --all --strict`: 15/15 items; `spec:gate`: PASS).
- `go vet ./...`, `go build ./...`, and `go test -race ./...` passed (including new tests in `internal/commands/cli_refinement_test.go` and `internal/routes/security_local_endpoints_test.go`).
- `bun run lint:all`, `bun run typecheck`, and `bun test` passed (380 pass, 23 skip, 0 fail).
- Redeployed `/home/jzhu/go/bin/omnillm`, restarted `omnillm.service` (active, healthy, database connected), and verified installed CLI behavior:
  - Removed dead startup flags rejected (`omnillm start --provider google` fails with `unknown flag: --provider`).
  - `--since` non-RFC3339 rejected (`usage --since 2026-11-15`), negative `--ttl` rejected (`settings set response-cache on --ttl -5`), extra positional args rejected (`provider list extra`).
  - `omnillm doctor`, `omnillm debug` (all provider tokens), and `omnillm completion --help` verified against the installed binary.
