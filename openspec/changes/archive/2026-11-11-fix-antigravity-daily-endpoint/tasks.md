## 1. Specification
- [x] 1.1 Run strict validation and obtain human approval of proposal, delta specs, design, and tasks before implementation.

## 2. Regression tests
- [x] 2.1 Add default and explicit endpoint tests, discovery-order coverage, and Gemini 3.8 catalog/effort-preservation assertions.

## 3. Implementation
- [x] 3.1 Switch the generation default and first discovery endpoint to the daily hostname.
- [x] 3.2 Add the three verified Gemini 3.8 Flash model variants.

## 4. Validation and deployment
- [x] 4.1 Run bun run spec:check, bun run lint:all, bun run typecheck, bun run test, and bun run build.
- [x] 4.2 Run go vet ./..., go build ./..., and go test -race ./....
- [x] 4.3 Verify deterministic five-call client-shape coverage and run configured isolated live clients or record concrete skip reasons.
- [x] 4.4 Append the exact daily hostname to the service bypass, validate configuration, rebuild/install, reload systemd, restart, and verify the process environment and health.
- [x] 4.5 Refresh affected model catalog views and test all three instances with fresh prompts; assert visible output and terminal completion for streaming and collected generation on the known-working account.
- [x] 4.6 Record verification outcomes and archive only when required checks and preceding tasks are complete.
