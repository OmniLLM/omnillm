## Approval and regression evidence

Strict validation passed for all 17 active-change/current-spec items before implementation. The user then approved the proposal, delta, design, tasks, service configuration/deployment, bounded live checks, and archival before committing and pushing the current branch to origin.

The new tests first failed against the previous code: default streaming and collected requests used the old production endpoint, discovery omitted the daily endpoint, and the three Gemini 3.8 variants were missing. After implementation, the targeted Antigravity/database/routes race checks passed.

Regression coverage verifies default and explicit generation endpoints, unchanged model effort/project/credential values, visible text and terminal completion in both modes, daily-first discovery with both retained fallbacks, and all three new catalog entries alongside existing models. The Gemini model token metadata follows the established catalog convention; no live maximum-token-limit claim is made.

## Automated verification

- `bun run spec:check`: strict validation and mandatory spec gate passed.
- `bun run lint:all` and `bun run typecheck`: passed.
- `bun run test`: 380 passed, 23 skipped, 0 failed.
- `bun run build`: passed; existing bundle-size advisory remains.
- `go vet ./...` and `go build ./...`: passed.
- `go test -race ./...`: passed across the complete repository.
- `git diff --check`: passed.
- `systemd-analyze verify` on the staged service unit: passed.

## Coding-client coverage

The complete Go suite includes `TestProviderLeasesPreserveClientToolLoops` and the provider/ingestion/serialization/route compatibility matrices. The deterministic client-shape loops assert five ordered calls, five observed results, and a terminal continuation without a sixth call:

| Client shape | Calls/results | Terminal result | Isolated native-client live smoke |
| --- | --- | --- | --- |
| Claude Code | 5 / 5 | Passed | Skipped: no isolated live manifest or credential inputs |
| Codex CLI | 5 / 5 | Passed | Skipped: no isolated live manifest or credential inputs |
| Droid | 5 / 5 | Passed | Skipped: no isolated live manifest or credential inputs |
| GitHub Copilot CLI custom provider | 5 / 5 | Passed | Skipped: no isolated live manifest or credential inputs |

All four executables are installed, but `scripts/live-model-matrix.json` is absent and no `OMNILLM_LIVE*` inputs are set. No normal runtime database or credential bundle was imported into a temporary live gateway. The regular-service generation probes below are separate from isolated native tool-loop testing.

## Deployment

The original service unit and installed binary were backed up under `/tmp/opencode/antigravity-deploy-s3ius0tj/` as `omnillm.service.backup` and `omnillm.binary.backup`. `make install` completed, the exact daily hostname was appended to `OMNILLM_NO_PROXY_APPEND`, and systemd was reloaded and `omnillm.service` restarted.

The first immediate environment check observed the startup wrapper before it executed the gateway. A subsequent check confirmed the actual gateway executable and both `NO_PROXY` and `no_proxy` contained `daily-cloudcode-pa.googleapis.com` alongside the existing entries. The HTTPS proxy remained configured for OAuth. The service was active and direct `/health` returned HTTP 200 at 2026-11-11 22:58 CST.

The host-specific service-unit change is outside the Git repository. Rollback uses the retained unit/binary backups followed by daemon reload and service restart; no credential migration is needed.

## Live generation results

Bounded checks started at 2026-11-11 22:58:30 CST. Each request used a fresh unique prompt and a 512-token output budget, avoiding replay of prior cached notices. Forced catalog refreshes returned 14 models for each instance, including all three Gemini 3.8 Flash effort variants.

| Instance | Model | Mode | Result |
| --- | --- | --- | --- |
| `antigravity` | `gemini-3.8-flash-low` | Collected | HTTP 200, exact `ANTIGRAVITY_OK` text, `stop` |
| `antigravity-2` | `gemini-3.8-flash-low` | Collected | HTTP 200, exact `ANTIGRAVITY_OK` text, `stop` |
| `antigravity-3` | `gemini-3.8-flash-low` | Collected | Gateway HTTP 502; correlated upstream HTTP 403 `PERMISSION_DENIED`, `VALIDATION_REQUIRED` |
| `antigravity` | `gemini-3.8-flash-low` | Streaming | HTTP 200, exact `ANTIGRAVITY_OK` text, `stop`, final `[DONE]` |

The third account requires Google account verification ("Verify your account to continue."). Its failure was not counted as generation success. The required known-working-account streaming/collected checks passed, and the remaining account condition was recorded independently as specified by the design. Medium/high effort variants have deterministic catalog and payload evidence; they were not separately live-probed.

## Archival completion

All preceding tasks and required checks passed. The validated OpenSpec archive command synchronized both provider requirements and archived this change on 2026-11-11. The archive commands were the only operations updating current-state requirement content. The two preceding Antigravity changes were also archived, with final verification updates documenting the resolved endpoint blocker and remaining account-verification condition.
