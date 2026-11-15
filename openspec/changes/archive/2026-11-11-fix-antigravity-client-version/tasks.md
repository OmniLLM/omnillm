## 1. Specification
- [x] 1.1 Run strict OpenSpec validation and obtain human approval of these artifacts before implementation.

## 2. Tests and implementation
- [x] 2.1 Add regression assertions for matching HTTP and JSON identities and preserved request content.
- [x] 2.2 Introduce the shared identity constant and use it at both generation request sites.

## 3. Verification
- [x] 3.1 Run bun run spec:check, bun run lint:all, bun run typecheck, bun run test, and bun run build.
- [x] 3.2 Run go vet ./..., go build ./..., and go test -race ./....
- [x] 3.3 Verify five-call deterministic client-shape coverage for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI; run isolated live client checks when configured or record concrete skip reasons.
- [x] 3.4 Rebuild and restart the service, test each Antigravity instance, and record genuine output separately from upgrade notices, quota errors, and account-verification failures.
- [x] 3.5 Archive only after all required verification succeeds and all preceding tasks are complete.
