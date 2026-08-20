# Design: Simplified Provider Operations and Identifiers

## Context

Provider creation is implemented by `POST
/api/admin/providers/auth-and-create/:type`; most re-authentication uses `POST
/api/admin/providers/:id/auth`; OpenAI and Antigravity use provider-specific
OAuth start and status endpoints. The CLI duplicates creation through root
`auth` and `provider add`, but does not expose a corresponding uniform re-login
command. Model commands live at the root even though every operation except
metadata lookup is scoped to a provider.

Provider instances persist `instance_id`, `name`, and `subtitle`. Runtime model
resolution currently indexes exact IDs and case-insensitive subtitles. The UI
and CLI already display subtitle as an alias in some places, so this change
standardizes the public terminology without introducing another stored field.

## Goals / Non-Goals

### Goals

- Give operators one obvious provider login command for create and re-auth.
- Keep provider model administration discoverable under the provider namespace.
- Resolve provider references consistently across CLI, admin API, and gateway
  model qualification.
- Preserve instance IDs, credentials, legacy commands, routes, and JSON fields.
- Fail safely when human-friendly metadata is not unique.

### Non-Goals

- Remove legacy commands or authentication endpoints in this change.
- Change provider-specific credential formats or OAuth protocol behavior.
- Rewrite existing instance IDs or add a database alias column.
- Allow aliases or names to silently choose among multiple providers.
- Change native slash-containing model precedence or provider failover ordering.

## Decisions

### One shared provider-reference resolver

Add a resolver at the provider-instance persistence/service boundary returning a
canonical instance ID or a typed unknown/ambiguous error. It builds complete
snapshots containing:

1. exact, case-sensitive instance IDs;
2. trimmed, case-insensitive aliases backed by `subtitle`;
3. trimmed, case-insensitive display names.

Precedence is ID, alias, then name. A lower-precedence match is never considered
after a higher-precedence match exists. Multiple matches at the selected level
produce a deterministic ambiguity error listing sorted instance IDs.

The same resolver is used by admin handlers and provider-prefixed model routing.
CLI commands send the reference rather than duplicating server resolution. Shell
completion displays ID, alias, and name but inserts an unambiguous reference.

### Alias is a compatibility projection over subtitle

No schema migration is needed. Internally, `ProviderInstanceRecord.Subtitle`
remains the storage field during this change. Public list/detail responses add
`alias` with the same value and retain `subtitle`. Metadata requests accept
either spelling; supplying both with different normalized values is a 400 error.
New CLI and UI text uses “alias,” and the CLI gains `--alias` while accepting
the hidden/deprecated `--subtitle` spelling.

### Normalize login orchestration without replacing provider protocols

Add authenticated login and flow-status admin operations with a normalized
envelope. The login request selects exactly one of:

- `type`: force creation of a new instance;
- `provider`: resolve and re-authenticate an existing instance;
- `subject`: resolve an existing provider first, otherwise treat a recognized
  provider type as creation.

If the subject is neither a provider reference nor a supported provider type,
the request fails as unknown. An explicit new-instance CLI option sends `type`
and avoids collisions.

The orchestration layer delegates to existing provider-specific setup and OAuth
primitives. A completed response contains `status: "complete"`, `provider_id`,
and `is_new`. Interactive flows contain `status: "pending"`, `provider_id`, a
random opaque `flow_id`, authorization URL/instructions, and optional user code.
The normalized status operation consumes only `flow_id` and returns pending,
complete, error, canceled, or expired. Flow IDs are time-bounded and contain no
credentials.

Legacy endpoints become adapters to the same orchestration where practical but
retain their exact external shapes. Provider-specific callback endpoints remain
because upstream redirect URI contracts require them.

### CLI compatibility through hidden shims

`provider login` owns authentication flags and behavior. `provider add` invokes
it with forced-new semantics. Root `auth` delegates to the same implementation.
`provider model` owns the existing model subcommands; root `model` remains a
hidden command pointing at the same command construction rather than copying
logic. This avoids registering one mutable Cobra command under two parents.

Ordinary help shows only `provider` and `virtualmodel` in the Providers group.
Deprecated invocations continue to work for scripts and receive no extra output
that would break parsers.

### Preserve native-model-first routing

The request model is still resolved in two stages. First, OmniLLM tries the
entire value as a native or virtual model. Only if that does not yield an
available model does it parse the first slash segment as a provider reference.
This preserves native identifiers such as `kimi/kimi-k3`. Once a prefix resolves,
only that segment is removed, so `name/vendor/model` forwards `vendor/model`.

Unlike current boolean lookup, the routing resolver retains ambiguity errors.
An ambiguous prefix terminates that fallback with a client-visible invalid
request and does not select or call one of the matches.

## Compatibility and Rollout

- Existing database rows need no migration; subtitle values immediately appear
  as aliases.
- Exact instance-ID commands and model strings behave unchanged.
- Existing alias model strings remain case-insensitive and unchanged.
- Root commands and old admin/OAuth routes remain available.
- Clients ignoring the additive `alias` response field are unaffected.
- Requests that previously relied on duplicate subtitles selecting the first
  database row will now fail explicitly; operators can disambiguate with ID or
  assign unique aliases.

## Failure Handling

- Unknown references return not found with the original reference.
- Ambiguous references return a client error with sorted matching instance IDs,
  never credentials or configuration.
- Existing-provider re-auth failure preserves the provider parent and existing
  durable metadata. Provider-specific credential replacement retains existing
  atomicity guarantees where available.
- New-provider failure follows existing parent-first rollback behavior.
- Expired, unknown, or already-consumed flow IDs cannot reveal prior results.

## Verification Strategy

Add table-driven resolver tests for precedence, case handling, ambiguity,
unknown references, and cache invalidation. Add route tests covering normalized
immediate and interactive login for create and re-auth, legacy endpoint
compatibility, and provider references on lifecycle/model operations. Add CLI
tests for the new hierarchy and hidden shims. Extend deterministic generation
tests across Chat Completions, Messages, and Responses for ID/name/alias
qualification, native namespace precedence, ambiguity, and preservation of
slash-containing model IDs. This changes request ingestion/routing, so the
repository's supported coding-agent compatibility regression policy applies.
