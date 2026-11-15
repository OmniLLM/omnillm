## 1. Specification and deterministic tests

- [x] 1.1 Strictly validate the OpenSpec artifacts and obtain human approval before implementation.
- [x] 1.2 Add deterministic Go CLI and server tests covering removed dead startup flags, `--allow-local-endpoints` opt-in, `--claude-code` output, RFC3339 `--since`/`--until` validation, negative `--ttl` rejection, strict `cobra.NoArgs`, `provider priorities --set` integer parsing, `virtualmodel update --description ""`, URL escaping, `logs -o json`, `debug` non-creation/multi-provider tokens, `doctor` hints, `completion` output writer, and completions.

## 2. Startup flags and server security wiring

- [x] 2.1 Remove dead no-op startup flags (`--account-type`, `--proxy-env`, `--provider`, `--console`) from `internal/commands/start.go` and `internal/server/server.go`, and update startup examples and docs.
- [x] 2.2 Wire `AllowLocalEndpoints` through `routes.SecurityOptions`, `RunServer`, provider auth handlers, `openaicompatprovider.Provider.SetupAuth`, and `provider login --allow-local-endpoints` without weakening default SSRF validation.
- [x] 2.3 Fix `--claude-code` output in `start`/`restart` for both foreground and background startup without printing `dummy` credentials or `0.0.0.0` URLs.

## 3. Subcommand validation, completions, diagnostics, and documentation

- [x] 3.1 Enforce `cobra.NoArgs` on zero-argument commands, validate RFC3339 `--since`/`--until` on `usage` (plus `responses` help and completions), reject negative `--ttl` on `settings set response-cache`, and fix `provider priorities --set` integer parsing.
- [x] 3.2 Fix `virtualmodel create`/`update`/`rename` (`MarkFlagRequired("upstream")`, `Flags().Changed(...)`, help text, URL escaping, completion), `config` (`ValidArgs`, URL escaping, `cmd.InOrStdin()`, `--enable-config-edit` help), and `logs` (`-o json` parity).
- [x] 3.3 Update `provider login` help/examples/completions, add `providerIDCompletionFunc` across `provider model` subcommands, allow TTY selection on `provider show`, clarify integer `<version>` in `model version set`, update `doctor`/`debug` hints and read-only DB inspection, fix `completion` `Use` and `cmd.OutOrStdout()`, and update `README.md` / `README.zh-CN.md`.

## 4. Verification

- [x] 4.1 Run `bun run spec:check`, `go vet ./...`, `go build ./...`, and `go test -race ./...` (plus relevant Bun checks for doc/repo integrity).
- [x] 4.2 Verify installed CLI help/behavior and record results before archiving.
