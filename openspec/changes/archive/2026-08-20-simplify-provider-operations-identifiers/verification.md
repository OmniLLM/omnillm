# Verification

## Isolated live validation

Built `cmd/omniproxy` and launched it with a fresh temporary home on
`127.0.0.1:55173`. The isolated database contained no user credentials.

- `/healthz` returned `OK`.
- `POST /api/admin/providers/login` with an API-key provider type returned
  `status: complete`, `is_new: true`, and canonical provider ID `google`.
- `provider rename google --alias livegoogle` succeeded.
- Provider list JSON emitted `alias: "livegoogle"` and the equal compatibility
  field `subtitle: "livegoogle"`.
- A metadata update addressed through uppercase alias `LIVEGOOGLE` resolved to
  canonical ID `google`.
- Model listing addressed through case-insensitive display name
  `Live Google Name` reached the intended provider. Upstream discovery returned
  the expected invalid-key response because the isolated smoke deliberately
  used a synthetic Google credential.
- `POST /api/admin/providers/login` with subject `LIVEGOOGLE` returned
  `status: complete`, `is_new: false`, and the unchanged canonical provider ID
  `google`, proving existing-provider re-authentication by alias.

## Deterministic coding-client compatibility

The maintained five-cycle compatibility route suites passed for Claude Code
Messages, Codex Responses, Droid Responses custom tools, and GitHub Copilot CLI
custom-provider shapes, including terminal continuation. Provider display-name
qualification also passed across Chat Completions, Messages, and Responses.

Installed clients were detected as Claude Code 2.1.237, Codex CLI 0.148.0,
Droid 0.199.0, and GitHub Copilot CLI 1.0.81-3. Bounded native-client live runs
were skipped because the isolated live-matrix manifest was absent and
`OMNILLM_LIVE_GITHUB_TOKEN` was unset; the synthetic provider used for the
separate admin smoke is not an authenticated model route.

## Automated verification

- `bun test tests/scripts/live-model-matrix.test.ts` — 12 passed, 0 failed
- `bun run spec:check`
- `bun run lint:all`
- `bun run typecheck`
- `bun test` — 361 passed, 22 environment-gated skipped, 0 failed
- `bun run build`
- `go vet ./...`
- `go build ./...`
- `go test -race ./...`

The initial full race run exposed an untracked asynchronous provider-registry
save overlapping a later test's global logger replacement. Registry background
saves are now tracked and waitable; the focused route race suite and complete
repository race suite pass after the fix.
