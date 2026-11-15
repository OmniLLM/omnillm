## Context

An audit of `main.go`, `internal/commands/`, `internal/server/server.go`, and `internal/routes/` showed that several CLI flags were dead leftovers (`--account-type`, `--proxy-env`, `--provider`, `--console`), `--allow-local-endpoints` and `--claude-code` were disconnected in `RunServer` / `startBackground`, and multiple commands lacked input validation (`--since`/`--until`, `--ttl`, `--set` integers, unexpected positional args) or URL path escaping. See `proposal.md` for full scope.

## Goals / Non-Goals

**Goals:** Ensure every documented CLI flag and parameter performs real work, validates bad input early, aligns with server behavior, and passes deterministic tests without altering existing gateway API contracts or caller invocations (`desktop`, `dev.ts`, `omni-dev.ts`, `entrypoint.sh`, `systemd`).

**Non-Goals:** Remove root persistent flags (`--server`, `--api-key`, `-o/--output`) from `rootCmd` (existing tests and callers rely on them); remove hidden compatibility shims (`auth`, `model`, `provider add`, `check-usage`) or `provider model toggle`; alter `parseUpstreamArgs` colon semantics beyond existing unit-tested behavior (`p1/m1:notanumber` must still error).

## Decisions

- **Remove dead startup flags rather than stubbing warnings**: `--account-type`, `--proxy-env`, `--provider`, and `--console` have no consumers in `internal/server/` or any caller in the repo (`desktop`, `dev.ts`, `omni-dev.ts`, `entrypoint.sh`, `systemd`). Removing them from `addStartFlags` and `StartOptions` prevents operators from assuming they configure runtime behavior.
- **Wire `--allow-local-endpoints` through `SecurityOptions`**: Add `AllowLocalEndpoints` to `routes.SecurityOptions`, set it in `RunServer`, apply it in `handleAuthAndCreateProvider` / `handleProviderAuth` when creating or re-authenticating `openai-compatible` providers, sync `p.allowLocal` in `openaicompatprovider.Provider.SetupAuth`, and expose `--allow-local-endpoints` on `provider login` (and hidden `provider add`). Default remains `false`, preserving SSRF rejection unless explicitly enabled.
- **Print `--claude-code` guidance in the foreground/CLI output safely**: Instead of printing `OPENAI_API_KEY=dummy` inside `_serve` (where stdout goes to `/dev/null` in background mode), emit guidance from the invoking `start`/`restart` command (after readiness in background mode, or before `RunServer` in foreground mode), rewriting `0.0.0.0` / `::` bind hosts to `127.0.0.1` for the client URL and referencing the configured API key path / `OMNILLM_API_KEY` rather than hardcoding `dummy` or echoing raw secrets.
- **Validate `--since` / `--until`, `--ttl`, `--set` priorities, and `cobra.NoArgs` client-side**: Parse `--since` / `--until` with `time.Parse(time.RFC3339, ...)` before querying `/api/admin/metering/*`, reject negative `--ttl` when `Flags().Changed("ttl")`, use `strconv.Atoi` instead of `fmt.Sscanf` in `provider priorities --set`, and add `Args: cobra.NoArgs` on zero-argument commands.
- **Keep `provider show` backward compatible while adding TTY selection**: Changing `providerShowCmd.Args` from `cobra.ExactArgs(1)` to `cobra.MaximumNArgs(1)` with `resolveProviderID` keeps `provider show <reference>` unchanged and aligns omitted-arg TTY selection with sibling `provider` commands.
- **Read-only `debug` inspection**: Check `os.Stat(dbPath)` before calling `database.InitializeDatabase(configDir)` in `debug` so inspecting an uninitialized machine does not create an empty SQLite file, and list tokens across all stored provider instances (`tokenStore.Get(inst.InstanceID)` or `GetAll`) rather than only `github-copilot`.

## Risks / Trade-offs

- [Removing unused startup flags could break an external script that passed `--provider` or `--account-type` as a no-op] → Verified zero callers in repo, desktop app, Docker entrypoint, dev runners, and systemd unit; documented in proposal as **BREAKING** and updated `README.md` / `README.zh-CN.md`.
- [Changing `provider.Use` or subcommand signatures could break existing unit tests] → Keep `modelEnableCmd.Use` (`enable <provider-id> <model-id>`) intact or update tests explicitly; leave `parseUpstreamArgs` strict so `TestParseUpstreamArgsInvalidWeightReturnsError` remains valid.
- [`provider login --api-key` still shadows root `--api-key`] → Required by the existing `TypeSafe CLI authentication` spec; clarify in `provider login` help that `--api-key` is the provider key while `OMNILLM_API_KEY` or root `--api-key` before `provider` sets the admin key.

## Migration Plan

Operators passing removed no-op startup flags (`--account-type`, `--proxy-env`, `--provider`, `--console`) must drop them from invocations. Operators enabling local OpenAI-compatible endpoints at startup now get true opt-in via `--allow-local-endpoints`. After approval and verification, rebuild and redeploy `omnillm` and restart the local systemd service if desired.
