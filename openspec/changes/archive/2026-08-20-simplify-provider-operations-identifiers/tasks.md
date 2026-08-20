# Tasks

## 1. Specification and Regression Coverage

- [x] 1.1 Strictly validate this OpenSpec proposal, delta specs, design, and
  tasks, and obtain human approval before implementation.
- [x] 1.2 Add table-driven provider-reference tests for ID/alias/name precedence,
  case handling, whitespace normalization, ambiguity, unknown values, sorted
  diagnostics, and snapshot invalidation after metadata changes.
- [x] 1.3 Add admin route tests for normalized synchronous and interactive login,
  new and existing providers, failure cleanup/preservation, flow expiry and
  consumption, provider references on scoped operations, alias/subtitle
  compatibility, and legacy authentication endpoints.
- [x] 1.4 Add CLI tests for `provider login`, forced-new login, nested provider
  model commands, provider references, completion, help visibility, and hidden
  legacy command behavior.
- [x] 1.5 Extend deterministic Chat Completions, Messages, and Responses routing
  tests for ID/name/alias qualification, precedence, ambiguous references,
  native namespace collisions, and slash-containing upstream model IDs.

## 2. Shared Provider Identity

- [x] 2.1 Implement one snapshot-based provider-reference resolver returning
  canonical IDs and typed deterministic unknown or ambiguous errors.
- [x] 2.2 Index exact IDs, case-insensitive persisted aliases, and
  case-insensitive display names, preserving ID then alias then name precedence.
- [x] 2.3 Invalidate the resolver snapshot for provider create, delete, rename,
  alias update, and provider-instance reload paths.
- [x] 2.4 Add `alias` to provider administration responses and accept alias
  metadata updates while retaining equal `subtitle` compatibility fields and
  rejecting conflicting dual-field requests.

## 3. Administration Login and Provider References

- [x] 3.1 Add normalized authenticated provider login and flow-status routes with
  common complete, pending, failed, canceled, and expired result envelopes.
- [x] 3.2 Route new-provider types and existing provider references through the
  current provider-specific immediate, device-code, and browser OAuth
  primitives without changing their protocol parameters.
- [x] 3.3 Preserve parent-first creation rollback and ensure failed existing
  re-authentication never deletes the existing provider aggregate.
- [x] 3.4 Resolve ID, alias, or display-name references for provider-scoped
  lifecycle, configuration, usage, priority, authentication, and model-catalog
  operations, returning canonical IDs and explicit ambiguity errors.
- [x] 3.5 Retain all legacy create, re-authentication, OAuth start/callback/status,
  and subtitle request/response contracts with regression coverage.

## 4. CLI Consolidation

- [x] 4.1 Add `provider login [type-or-provider]` with interactive selection,
  forced-new semantics, all existing auth flags, and normalized flow polling.
- [x] 4.2 Move the canonical model command tree under `provider model` and update
  help, examples, completion, and human-readable labels to use provider
  references and aliases.
- [x] 4.3 Retain hidden root `auth`, root `model`, and `provider add` shims that
  delegate to the canonical implementations without duplicating Cobra command
  instances or changing script-visible output.
- [x] 4.4 Rename CLI `--subtitle` presentation to `--alias` while accepting the
  hidden/deprecated spelling and rejecting conflicting values.

## 5. Gateway Provider-Qualified Models

- [x] 5.1 Use the shared resolver for provider-qualified model fallback while
  preserving full-native-model resolution before prefix interpretation.
- [x] 5.2 Propagate ambiguous provider references as structured client errors and
  ensure no matching provider is dispatched.
- [x] 5.3 Preserve only-prefix removal for native upstream model identifiers with
  one or more remaining slash segments.
- [x] 5.4 Verify provider-reference behavior is contract-equivalent across OpenAI
  Chat Completions, Anthropic Messages, and OpenAI Responses routes.

## 6. Client Compatibility Verification

- [x] 6.1 Run deterministic multi-turn tool-use regression coverage for Claude
  Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider request shapes.
- [x] 6.2 When each supported client and local model configuration is available,
  run a bounded live smoke test with at least five sequential native tool calls,
  record sanitized call/result counts and terminal evidence, and record each
  skipped client with the concrete reason.

## 7. Verification and Archive

- [x] 7.1 Run focused Go tests for provider identity, admin authentication, CLI
  commands, provider persistence, and all generation routing dialects under the
  race detector where applicable.
- [x] 7.2 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`,
  `bun test`, and `bun run build`.
- [x] 7.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 7.4 Check every task only after completion, then archive with `bunx openspec
  archive simplify-provider-operations-identifiers --yes` and confirm archive is
  the only operation that updates current-state specs.
