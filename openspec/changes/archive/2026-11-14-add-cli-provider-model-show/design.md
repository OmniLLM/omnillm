## Context

The provider list API returns runtime instance summaries, potentially including
provider configuration. The provider model list API returns a catalog with
model details, but no provider or model detail endpoint exists. Existing CLI
reads support `--output json`; the hidden root model shim dispatches to the
canonical provider model commands.

## Goals / Non-Goals

**Goals:** Reuse current authenticated list APIs, preserve existing commands,
and make detail output safe and deterministic.

**Non-Goals:** Add or change admin API endpoints, expose credentials, fetch
unrelated pricing metadata, or change provider/model discovery and persistence.

## Decisions

1. Implement `provider show` by selecting from the existing provider list. Match
   exact ID first, then unique normalized alias, then unique normalized display
   name; report sorted IDs for ambiguity. The server has no provider GET endpoint,
   so this avoids a new public API. The CLI must mirror the documented reference
   resolver and cover its precedence with tests.
2. Implement `provider model show` using the existing provider-scoped model list
   endpoint, allowing the server to resolve the provider reference. Filter the
   returned catalog by exact model ID without embedding that ID in a URL. This
   naturally preserves namespaced IDs and avoids new API routes.
3. Use a provider field allowlist for both JSON and table output: `id`, `type`,
   `name`, `alias`, `subtitle`, `authStatus`, `isActive`, `totalModelCount`, and
   `enabledModelCount`. Never print the `config` object or arbitrary extra
   fields; unknown provider types can return unfiltered configuration. JSON is
   therefore a selected, safe detail object rather than the raw list response.
   For models, output the selected catalog object in JSON and render available
   known fields in table mode.
4. Require the provider and model arguments in noninteractive detail commands;
   register provider completion and document usage. The legacy root model shim
   will dispatch to the new canonical subcommand as it does for existing ones.

## Risks / Trade-offs

- Provider reference logic could drift from the server resolver → test exact ID,
  case-insensitive alias/name, whitespace, precedence, ambiguity and unknown
  cases; keep matching behavior aligned with the current provider specification.
- Safe provider JSON omits configuration fields that some operators may want →
  intentionally prefer credential safety; existing list output is unchanged.
- Model catalog contents can be cached or degraded → show available fields
  without inventing unknown values; preserve existing API errors.
- JSON selection is necessarily client-side because the server supplies lists →
  document that show returns one object, while existing list commands stay raw.

## Migration Plan

Additive CLI commands require no migration. Roll back by removing the new
commands; no persisted data or server API changes are made.
