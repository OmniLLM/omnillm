## 1. OpenSpec foundation

- [x] 1.1 Pin the OpenSpec CLI and add local validation, gate, check, and update scripts.
- [x] 1.2 Initialize repository-local OpenSpec configuration and generated Claude Code skills.
- [x] 1.3 Create and strictly structure the baseline proposal and cross-cutting design.

## 2. Brownfield capability baseline

- [x] 2.1 Capture and verify gateway API, CIF translation, routing/failover, and caching requirements.
- [x] 2.2 Capture and verify provider, persistence, and security requirements.
- [x] 2.3 Capture and verify admin API/UI, CLI/operations, desktop, and governance requirements.
- [x] 2.4 Run strict validation and correct every baseline requirement or scenario error.

## 3. Mandatory governance

- [x] 3.1 Preserve the worktree policy and add mandatory SDD instructions to `CLAUDE.md` and `AGENTS.md`.
- [x] 3.2 Add contributor and pull-request workflow documentation.
- [x] 3.3 Add README links and a non-normative documentation authority map.
- [x] 3.4 Move dated plans and completed fix/report narratives to `docs/archive/` without deleting history.

## 4. Automated enforcement

- [x] 4.1 Implement the diff-aware Bun specification gate with base resolution, classification, artifact checks, archive integrity, and emergency exemption handling.
- [x] 4.2 Add specification-gate unit tests and governance regression tests.
- [x] 4.3 Add a full-history CI specification job and make existing Bun and Go jobs depend on it.

## 5. Verification and archive

- [x] 5.1 Run `bun run spec:validate` and `bun run spec:gate` successfully.
- [x] 5.2 Run `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build` successfully.
- [x] 5.3 Run `go vet ./...`, `go build ./...`, and `go test -race ./...` successfully.
- [x] 5.4 Check every completed task and archive the baseline change so its deltas become current-state specs.
- [x] 5.5 Re-run strict validation, governance tests, and the gate against the archived state.
