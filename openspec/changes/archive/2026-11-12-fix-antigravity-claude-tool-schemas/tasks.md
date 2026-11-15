## 1. Specification and approval

- [x] 1.1 Complete proposal, deltas, design, and sanitized investigation; run strict OpenSpec validation and obtain human approval before implementation.
- [x] 1.2 Revalidate and obtain approval for the observed tool-identity/continuation extension before implementing it.

## 2. Regression coverage

- [x] 2.1 Add failing Antigravity Claude schema transport tests for the nested-union reproduction, retained constants/references/constraints, both Claude models, three ingestion dialects, streaming/collected execution, immutability, nil schemas, and unchanged Gemini behavior.
- [x] 2.2 Add wrapped-error fallback regression tests and route-level assertions that all three streaming dialects do not replay explicitly disallowed errors; retain allowed fallback and authentication/cancellation coverage.
- [x] 2.3 Add failing regressions for native/fallback tool IDs, call/result replay and missing-name resolution, five-call histories for all client shapes through the Antigravity converter, and tool-use stop semantics.

## 3. Implementation

- [x] 3.1 Select full JSON Schema transport for Antigravity Claude tools while preserving other model and request semantics.
- [x] 3.2 Implement Antigravity HTTP-error fallback eligibility and honor it in shared route classification without changing public error status/envelopes.
- [x] 3.3 Preserve Antigravity call/result IDs and missing result names, retain native output IDs, and emit tool-use stop semantics when calls are present.

## 4. Verification and deployment

- [x] 4.1 Run bun run spec:check, bun run lint:all, bun run typecheck, bun run test, bun run build, go vet ./..., go build ./..., and go test -race ./....
- [x] 4.2 Verify deterministic five-call/five-result terminal tool loops for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider shapes.
- [x] 4.3 Build an isolated fresh-port gateway with explicitly approved temporary credentials; verify both Claude models using real Claude Code with SendMessage present and at least five sequential read-only native tool calls, observed results, terminal output, and successful exit.
- [x] 4.4 Run corresponding available native-client checks for Codex, Droid, and GitHub Copilot CLI or record concrete missing prerequisites; never label a failed attempted loop as skipped.
- [x] 4.5 Back up the installed binary, install/restart OmniLLM, confirm health, and repeat bounded native Claude Code verification on both installed-service model routes.
- [x] 4.6 Record sanitized results and remove temporary credential state; archive only after every preceding task and required check succeeds.
