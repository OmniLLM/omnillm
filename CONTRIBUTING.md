# Contributing to OmniLLM

OmniLLM uses OpenSpec for mandatory spec-driven development. Current behavior is defined under `openspec/specs/`; other documentation is explanatory or historical.

## Required workflow

1. Run `bunx openspec new change <change-id>`.
2. Write `proposal.md` and identify every affected capability.
3. Write delta specs with normative requirements and testable scenarios.
4. Write `design.md` for cross-capability work, provider additions, or changes to public APIs, CIF, routing, persistence, or security.
5. Write `tasks.md`, placing specification and test work before implementation.
6. Run `bun run spec:validate` and obtain human approval before changing code.
7. Implement only the approved tasks and check items as they are completed and verified.
8. Run `bun run spec:check` plus all relevant Bun and Go checks.
9. Archive the completed change with `bunx openspec archive <change-id> --yes` so the current-state specs are updated.

Do not directly edit requirement content under `openspec/specs/`. A change to code, tests, dependencies, or build/runtime configuration without a complete active or archived OpenSpec change is rejected by CI.

## Verification

The complete repository checks are:

```sh
bun run spec:check
bun run lint:all
bun run typecheck
bun test
bun run build
go vet ./...
go build ./...
go test -race ./...
```

Run the subset relevant while developing, then the complete suite before archive.

## Pull requests

Include the change ID, explain the affected capabilities, show strict validation evidence, and state whether the change is active or archived. The GitHub `spec` status check must be required on the protected `master` branch.

## Emergency exemption

An urgent security fix or revert may include `SPEC-EXEMPT: <substantive reason>` in the HEAD commit message. The reason must be at least 15 characters, is emitted prominently by CI, and does not bypass validation of existing specs. Open a follow-up OpenSpec change in the next pull request. Routine fixes, refactors, dependency updates, and test changes are not eligible.
