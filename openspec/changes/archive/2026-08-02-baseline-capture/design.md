## Context

See `proposal.md` for motivation. OmniLLM is a brownfield monorepo with a Go gateway, Bun/React administration console, Tauri desktop shell, SQLite persistence, and behavior recorded in code, tests, operator docs, dated designs, and provider regression notes. The repository already uses Bun in CI and has no Python toolchain. The initial OpenSpec catalog is empty, so the conversion must establish both the baseline and the process that maintains it.

## Goals / Non-Goals

**Goals:**
- Make OpenSpec the normative, cumulative description of observable system behavior.
- Capture a trustworthy baseline without intentionally changing runtime behavior.
- Require a validated and approved change proposal before code implementation.
- Enforce the requirement in local tooling and CI, not only in prose.
- Preserve historical design rationale while clearly separating it from normative requirements.

**Non-Goals:**
- Redesigning APIs, providers, routing, storage, UI, CLI, or desktop behavior during baseline capture.
- Exhaustively documenting private implementation details that can change without observable impact.
- Deleting historical documentation.
- Treating local Git hooks as the security boundary; CI remains authoritative.

## Decisions

### Use OpenSpec rather than Spec Kit

OpenSpec's proposal and delta-spec lifecycle accumulates current-state specifications through archive, directly matching the requirement that future code changes update the retained system specification. Its strict CLI validation fits the existing Bun CI. Spec Kit was considered, but its workflow is optimized for greenfield feature artifacts, adds a Python/uv toolchain, and lacks an equivalent cumulative archive model and straightforward repository-wide CI validator.

### Pin OpenSpec in the repository

`@fission-ai/openspec` is an exact devDependency and all commands run through package scripts. This keeps contributor and CI validation semantics aligned and makes tool upgrades explicit reviewable changes. Generated OpenSpec files and managed instruction blocks are regenerated with `openspec update`, not manually maintained.

### Capture the baseline as one explicit exception

The initial conversion writes the twelve current-state capabilities by archiving a single baseline change. Because there is no prior current-state catalog, the baseline change's ADDED deltas are the authoritative seed and archive creates `openspec/specs/**`. After this change, direct edits to current-state requirement content are forbidden: every modification must originate in an active change delta and reach the catalog through archive.

### Specify behavior by capability

The catalog is divided into gateway, translation, provider, routing, control-plane, persistence, cache, operations, security, desktop, and governance capabilities. Requirements describe externally observable contracts and testable constraints, while internal package names and execution steps remain in design/tasks. Existing tests and implementation are the primary evidence; living docs and regression reports supply additional cases that must be verified against code.

### Make SDD strict at three layers

1. `CLAUDE.md`, root `AGENTS.md`, generated OpenSpec guidance, and `CONTRIBUTING.md` define the mandatory human/agent sequence.
2. A Bun diff gate checks that code changes include a complete active change or a same-branch archive, validates artifact structure, blocks direct catalog edits, and rejects archived unchecked tasks.
3. A named GitHub Actions job runs strict validation and the gate before the existing Bun and Go jobs; branch protection must require this job.

This compensates for OpenSpec's intentionally flexible default workflow and its warning-only treatment of incomplete archived tasks.

### Compare changes against a real merge base

The gate resolves its base from `SPEC_GATE_BASE`, PR base branch, push-before SHA, local `origin/master`, local `master`, then `HEAD~1`. CI checkouts fetch full history. If no valid base can be resolved, the gate fails closed with an actionable message rather than silently allowing an unverified change.

### Classify behavior-affecting files conservatively

Go, frontend, desktop, scripts, tests, dependency manifests/locks, build files, and build-affecting workflows trigger the spec requirement. Markdown, OpenSpec artifacts, governance documents, and non-build repository metadata do not. Docs-only and proposal-only commits therefore remain possible, while dependency, test, build, and runtime changes remain accountable.

### Provide a narrow emergency escape hatch

A commit containing `SPEC-EXEMPT: <substantive reason>` may bypass the change-artifact requirement for an urgent security fix or revert. The gate still validates existing specs, prints the exemption prominently, and contributor policy requires a follow-up spec change. This avoids blocking emergency recovery without creating a silent bypass.

### Preserve historical documentation as non-normative

Living guides remain in place; completed plans and point-in-time reports move with history to `docs/archive/`. A docs index maps each resource to its owning current-state capability and banners clarify that OpenSpec is authoritative. No historical information is deleted.

## Risks / Trade-offs

- **Baseline describes aspirations instead of reality** → Cross-check every scenario against code/tests; record unsupported claims as known gaps rather than requirements.
- **Specs become too broad to maintain** → Keep requirements behavioral and divide ownership across twelve stable capabilities.
- **Diff gate produces false positives** → Unit-test classification and base resolution, exempt docs/spec-only changes, and emit precise remediation.
- **Diff gate can be bypassed locally** → Treat local execution as feedback; require the named CI job through branch protection.
- **Urgent fixes are delayed** → Allow only a loud, reasoned exemption with mandatory follow-up.
- **OpenSpec behavior drifts between environments** → Pin the exact package version and use the frozen lockfile in CI.
- **Generated guidance is accidentally corrupted** → Prohibit manual edits and guard generated paths/tool-version changes in tests and the gate.
- **Historical docs contradict current specs** → Mark them non-normative and maintain an authority map.
- **Initial conversion is large** → Parallelize evidence gathering by capability, validate incrementally, and make no runtime refactors in this change.

## Migration Plan

1. Install and initialize the pinned OpenSpec toolchain.
2. Create and strictly validate proposal, capability deltas, design, and tasks for this baseline change.
3. Add governance instructions, current project context, the diff gate, tests, and CI wiring.
4. Archive the completed baseline change, creating the current-state capability catalog.
5. Re-run strict validation and all existing Bun and Go checks.
6. Configure the GitHub `spec` job as a required branch-protection check after merge.

Rollback is a normal git revert of the conversion commit. Runtime behavior is not intentionally changed, so rollback does not require data migration; it removes only development governance and specification assets.
