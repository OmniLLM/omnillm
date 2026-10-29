## Approval

The user approved this proposal, delta spec, design, tasks, implementation, testing, and deployment in the conversation on 2026-10-29. Strict validation passed before implementation.

## Deterministic checks

- Regression-first run failed on the original code for RGB tuples, trailing-item policies, nested tuples, keyword-like property names, and metadata/schema ingestion.
- Focused ingestion and real Copilot adapter/fake-upstream continuation regressions passed after implementation.
- `go vet ./...`, `go build ./...`, and `go test -race ./...` passed.
- Existing client cache replay and agentic route matrices passed, retaining five-turn fixtures for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI.
- `bun run lint:all`, `bun run typecheck`, `bun run build`, `bun test`, and `bun run spec:check` passed. Bun reported 373 passes, 22 existing skips, and zero failures. The frontend build reported its existing large-chunk advisory.
- `git diff --check` passed.

## Isolated live proof

A temporary candidate gateway ran on automatically allocated loopback port 44691 with an isolated home and database, limited to the required existing provider credential references. Clients used temporary homes and a harmless five-file chain; each file revealed only the next path, requiring sequential observation. Temporary credential-bearing gateway/client homes were removed after the run. The ordinary service was not used for these client checks.

| Client | Route/model | Calls | Observed results | Terminal marker | Exit |
| --- | --- | --- | --- | --- | --- |
| Claude Code | Messages / claude-sonnet-5.5 | 5 | 5 | present | 0 |
| Codex CLI | Responses / gpt-6.1-sol | 5 | 5 | present | 0 |
| Droid | Messages / claude-sonnet-5.5 | 5 | 5 | present | 0 |
| GitHub Copilot CLI custom provider | Messages / claude-sonnet-5.5 | 5 | 5 | present | 0 |

Event records verified alternating calls/results, matching relationships where exposed, successful tool completion, no sixth call, terminal continuation, and successful process exit. No client live check was skipped. No delegation or subagents were used.

The original unmodified color-harmony legacy schema was also accepted on the candidate gateway and reached `end_turn`, without executing a tool action.

## Deployment

Installed binary matches the verified candidate SHA-256 `0b8aeda6807e710f84fdabac26a2b3a6840832a78db3d5a17689eb3b6ee7be12`. Restarted `omnillm.service`; the service is active/running with zero automatic restarts. Saved the prior binary at `/home/jzhu/scripts/omnillm-schema-diagnostic/omnillm-before-tuple-fix` for rollback.

Post-restart requests use the production Sonnet route with tool execution disabled and verify terminal output, rather than relying on health probes alone. Both the original single color-harmony tool and the complete 170-tool captured set returned HTTP 200, `stop_reason: end_turn`, and `OK.`. Neither request executed a tool action.

## Limits

The historical failing tool indexes cannot all be reconstructed from metadata-only production logs. The fresh captured tool set contains 170 tools; other sessions can have different sets. The separate unavailable model spelling `claude-opus-5-5` remains outside this schema change; the available spelling is `claude-opus-5.5`.
