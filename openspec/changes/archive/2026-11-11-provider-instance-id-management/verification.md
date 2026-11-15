## Automated checks

- `bun run spec:check`: passed, including strict OpenSpec validation and the spec gate.
- `bun run lint:all`, `bun run typecheck`, `bun test --timeout 30000 tests/`, and `bun run build`: passed. Bun reported 380 passing and 23 skipped tests; the dangerous visual suite and server-dependent integration tests remain intentionally skipped. The build reported a large-chunk advisory.
- `OMNILLM_UI_BROWSER_TEST=1 bun test tests/frontend/provider-identity-browser.test.ts`: passed in headless Chrome, covering validation, cancellation, confirmation, a retryable failure, successful reload, and existing name/alias controls.
- `go vet ./...`, `go build ./...`, and `go test -race ./...`: passed.
- `TestProviderLeasesPreserveClientToolLoops`: passed for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider request shapes. Each fixture issued five ordered tool calls, carried five associated observed results, and received one terminal continuation with no sixth call. Streaming lease coverage also passed for Chat Completions, Messages, and Responses.

## Live client smoke availability

Claude Code, Codex CLI, Droid, and GitHub Copilot CLI executables are installed, but no isolated live model matrix configuration is present and the explicit `OMNILLM_LIVE_GITHUB_TOKEN`, `OMNILLM_LIVE_GOOGLE_API_KEY`, `OMNILLM_LIVE_OPENAI_TOKEN_BUNDLE`, `OMNILLM_LIVE_OPENAI_API_KEY`, and `OMNILLM_LIVE_ALIBABA_API_KEY` inputs are unset. The four live smokes were therefore skipped; no live call/result counts or terminal evidence are claimed. Existing user gateway credentials and configuration were not reused.
