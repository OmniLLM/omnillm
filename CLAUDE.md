# Worktree cleanup

After completing work in a temporary worktree you created specifically for the task, leave and remove that worktree only after its changes are safely preserved, such as by committing, merging, or explicitly retaining them.

Never remove a pre-existing or user-created worktree, and never discard uncommitted or unmerged changes. If safe removal cannot be confirmed, keep the worktree and report the remaining cleanup.

# Spec-Driven Development (mandatory)

OmniLLM uses OpenSpec as the normative source of truth. Every change to code, tests, dependencies, build/runtime configuration, or behavior MUST have a corresponding OpenSpec change in the same branch or pull request. Documentation-only and proposal-only changes are exempt from creating a second change.

Follow this sequence without skipping or reordering steps:

1. Create `openspec/changes/<change-id>/proposal.md` describing why and what changes.
2. Add delta specs at `openspec/changes/<change-id>/specs/<capability>/spec.md`. Use `ADDED`, `MODIFIED`, `REMOVED`, or `RENAMED` requirements, and give every requirement at least one `#### Scenario:`.
3. Add `design.md` when the change touches multiple capabilities, adds a provider, or changes a public API, CIF, routing, persistence, or security contract.
4. Add an ordered `tasks.md` checklist.
5. Run `bun run spec:validate`. Do not write implementation code until strict validation passes and a human approves the proposal, specs, design, and tasks.
6. Implement the approved tasks, checking each item only after it is complete and verified.
7. Run `bun run spec:check` and all relevant Bun and Go checks.
8. Archive with `bunx openspec archive <change-id> --yes` only after every task is checked and all verification passes.

Never directly edit requirement content under `openspec/specs/**`; update it through a validated delta and archive. The initial `2026-08-02-baseline-capture` is the only direct-seeding exception. Never hand-edit generated OpenSpec skill files; regenerate them with `bun run spec:update`.

The CI `spec` job and `scripts/spec-gate.ts` enforce this policy. `SPEC-EXEMPT: <substantive reason>` is reserved for urgent security fixes and reverts, is printed prominently in CI, and requires a follow-up spec change.
