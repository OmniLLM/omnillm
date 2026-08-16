# Verification Evidence

## Approved specification

- Change: `add-provider-prompt-caching`
- Strict validation: `bun run spec:validate`
- Result: 14 passed, 0 failed
- Human approval: granted before implementation on 2026-08-16

## Baseline live matrix

### Smoke

- Command: `OMNILLM_RUN_LIVE_MATRIX=1 OMNILLM_LIVE_MATRIX_REPORT_DIR=.tmp-live-tests/reports/prompt-cache-baseline-smoke bun run test:model-matrix:live:smoke`
- Result: 0 passed, 0 failed, 21 skipped, 3 not applicable
- Skip reason: configured Copilot rows require `OMNILLM_LIVE_GITHUB_TOKEN`, which is unavailable in this process
- Report: `.tmp-live-tests/reports/prompt-cache-baseline-smoke/live-model-matrix-smoke-1786868487848.json`
- Port: no gateway was launched because every runnable row was skipped before provisioning

### Extended

- Command: `OMNILLM_RUN_LIVE_MATRIX=1 OMNILLM_LIVE_MATRIX_REPORT_DIR=.tmp-live-tests/reports/prompt-cache-baseline-extended bun run test:model-matrix:live:extended`
- Result: 0 passed, 0 failed, 33 skipped, 21 not applicable
- Skip reason: configured Copilot rows require `OMNILLM_LIVE_GITHUB_TOKEN`, which is unavailable in this process
- Report: `.tmp-live-tests/reports/prompt-cache-baseline-extended/live-model-matrix-extended-1786868531902.json`
- Port: no gateway was launched because every runnable row was skipped before provisioning

## Post-change live matrix

### Disabled guard

- Command: `bun run test:model-matrix:live`
- Result: exited successfully before credentials, build, state, port allocation, or network access

### Smoke

- Command: `OMNILLM_RUN_LIVE_MATRIX=1 OMNILLM_LIVE_MATRIX_REPORT_DIR=.tmp-live-tests/reports/prompt-cache-post-smoke bun run test:model-matrix:live:smoke`
- Result: 0 passed, 0 failed, 21 skipped, 3 not applicable
- Skip reason: `OMNILLM_LIVE_GITHUB_TOKEN` unavailable
- Report: `.tmp-live-tests/reports/prompt-cache-post-smoke/live-model-matrix-smoke-1786874021280.json`
- Port: no gateway was launched because every runnable row was skipped before provisioning

### Extended

- Command: `OMNILLM_RUN_LIVE_MATRIX=1 OMNILLM_LIVE_MATRIX_REPORT_DIR=.tmp-live-tests/reports/prompt-cache-post-extended bun run test:model-matrix:live:extended`
- Result: 0 passed, 0 failed, 33 skipped, 27 not applicable
- Prompt-cache rows: not applicable because cache-read usage is not verified for the configured Copilot Grok account/model contract
- Remaining skip reason: `OMNILLM_LIVE_GITHUB_TOKEN` unavailable
- Report: `.tmp-live-tests/reports/prompt-cache-post-extended/live-model-matrix-extended-1786874046222.json`
- Port: no gateway was launched because every runnable row was skipped before provisioning

## Additional credentialed fresh-port attempts

- Personal GitHub CLI token smoke: launched isolated gateway at `127.0.0.1:39084`; all 21 runnable rows failed during Copilot provider provisioning with HTTP 500, so no model/cache request executed. Report: `.tmp-live-tests/reports/prompt-cache-credentialed-post-smoke/live-model-matrix-smoke-1786879246880.json`.
- Corporate GitHub CLI token smoke: launched a second isolated gateway at `127.0.0.1:38614`; all 21 runnable rows likewise failed during Copilot provider provisioning with HTTP 500. Report: `.tmp-live-tests/reports/prompt-cache-corporate-post-smoke/live-model-matrix-smoke-1786879325194.json`.
- Authenticated OmniLauncher Copilot smoke: refreshed the existing long-lived Copilot credential without printing it, launched a third isolated gateway at `127.0.0.1:38242`, and passed 21 applicable rows with 0 failures and 3 declared not-applicable Grok 4.6 tool rows. Report: `.tmp-live-tests/reports/prompt-cache-omnilauncher-post-smoke/live-model-matrix-smoke-1786879675888.json`.
- Authenticated OmniLauncher Copilot extended matrix: launched another isolated gateway at `127.0.0.1:39232` and passed all 33 applicable rows with 0 failures and 27 explicit not-applicable rows, including five sequential tool cycles, terminal continuation, large results, cancellation, and declared provider limitations. Report: `.tmp-live-tests/reports/prompt-cache-omnilauncher-post-extended/live-model-matrix-extended-1786880067802.json`.
- These supplied-credential failures and successes are recorded faithfully. Deterministic fake-upstream and compatibility tests remain green.

## Deterministic and build verification

- `bun run spec:check`: pass
- `bun run lint:all`: pass; dependency freshness warning only
- `bun run typecheck`: pass
- `bun test`: 358 passed, 22 skipped, 0 failed
- `bun run build`: pass; existing chunk-size warning only
- `go test ./...`: pass
- `go test -race ./...`: pass
- `go vet ./...`: pass
- `go build ./...`: pass
- Focused prompt-cache ingestion, provider payload, streaming usage, serializer, database, affinity, route, and live-runner tests: pass

## Native coding clients

- Claude Code: skipped because no authenticated model route and reusable isolated native-client runner are available in this process
- Codex CLI: skipped because no authenticated model route and reusable isolated native-client runner are available in this process
- Droid: skipped because no authenticated model route and reusable isolated native-client runner are available in this process
- GitHub Copilot CLI custom provider: skipped because `OMNILLM_LIVE_GITHUB_TOKEN` and a safe temporary custom-provider runner are unavailable
- Deterministic Chat, Messages, Responses, Codex custom-tool, and Droid custom-tool five-call fixtures remain covered by the Go compatibility suites
