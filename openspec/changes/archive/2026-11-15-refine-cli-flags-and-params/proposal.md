## Why

An audit of the `omnillm` CLI found several dead startup flags, broken or misleading helper flags, silent validation gaps, and stale diagnostics/documentation. Cleaning up these flags and parameters makes every exposed CLI option accurate, useful, and predictable.

## What Changes

- **BREAKING:** Remove dead no-op startup flags (`--account-type`/`-a`, `--proxy-env`, `--provider`, `--console`) from `omnillm start` and `omnillm restart`.
- Wire `start`/`restart --allow-local-endpoints` into server security options (and add `--allow-local-endpoints` to `provider login`) so explicit local-endpoint opt-in works end-to-end without weakening SSRF defaults.
- Fix `--claude-code` (`-c`) on `start`/`restart` so it prints usable client environment instructions in both foreground and background startup modes without emitting bogus `dummy` API keys or `0.0.0.0` URLs.
- Enforce strict CLI parameter validation and safer URL/flag handling across subcommands:
  - Validate `--since`/`--until` RFC3339 timestamps on `usage` (and document `responses` under `--api-shape`).
  - Reject negative `--ttl` values passed explicitly to `settings set response-cache`.
  - Reject trailing non-numeric characters in `provider priorities --set`.
  - Enforce `cobra.NoArgs` on zero-argument commands (`provider list`, `provider priorities`, `virtualmodel list`, `config list`, `settings get *`, `settings clear-response-cache`, `status`, `status auth`, `usage`, `check-usage`, `doctor`, `debug`, `sync-names`).
  - Require `--upstream` on `virtualmodel create`, honor `Flags().Changed` on `virtualmodel update` (allowing `--description ""`), improve `virtualmodel update` flag help, URL-escape IDs in `virtualmodel` and `config` endpoints, read `cmd.InOrStdin()` in `config set --stdin`, and honor `-o json` on `logs`/`logs tail`.
- Improve CLI diagnostics, completions, and help text:
  - Fix `provider login` flag help (`--region`, `--endpoint`, `--method`, `--plan`), add supported-type help/examples and completions, and add `providerIDCompletionFunc` across `provider model` subcommands plus `virtualmodel rename` and `config` names.
  - Allow `provider show [reference]` interactive TTY selection (`MaximumNArgs(1)`) consistent with sibling `provider` subcommands, and clarify integer `<version>` in `provider model version set`.
  - Update `doctor` and `debug` to reference `omnillm provider login` instead of deprecated `omnillm auth`, inspect all provider tokens in `debug`, and avoid creating an empty SQLite database in `debug` when none exists.
  - Fix `completion` `Use` syntax (`<bash|zsh|fish|powershell>`) and write output to `cmd.OutOrStdout()`.
  - Update `README.md` and `README.zh-CN.md` to remove obsolete CLI examples and dead flags.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-ops-config`: Startup flag cleanup and wiring, strict parameter validation, completion/help accuracy, and updated diagnostics.

## Impact

Affected code is in `internal/commands/`, `internal/server/`, `internal/routes/` (`SecurityOptions` local-endpoint wiring), `internal/providers/openaicompatprovider/` (`SetupAuth` local-endpoint flag sync), `README.md`, `README.zh-CN.md`, and OpenSpec artifacts. Existing callers using `start --port/--host/--api-key/--verbose/--enable-config-edit/--foreground` (`desktop`, `dev.ts`, `omni-dev.ts`, `entrypoint.sh`, `systemd`) remain unaffected.
