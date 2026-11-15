# Verification

- Strict OpenSpec validation passed before the proposal, delta, design, and
  tasks were approved by the operator.
- `go test ./internal/commands -count=1` passed. Tests cover both detail
  formats, credential exclusion, resolution precedence, ambiguity and missing
  cases, slash-containing model IDs, help, and legacy command compatibility.
- A local mock HTTP server and the real CLI verified `provider show`, `provider
  model show`, and legacy `model show` with `--output json`; each returned the
  expected single object and the provider output excluded a synthetic secret.
- `bun run spec:check`, `bun run lint:all --no-cache`, `bun run typecheck`,
  `go vet ./...`, `go build ./...`, and `go test -race ./...` passed.
- `bun test` passed with 380 tests, 23 skipped, and zero failures. The dangerous
  visual suite was not enabled, and a server-dependent integration was skipped
  because no server was running.
- `bun run build --outDir /tmp/opencode/omnillm-show-build.HKB4kb` passed; the
  temporary output avoided overwriting the existing ignored `pages/admin/`
  directory. Vite reported a chunk-size advisory and ESLint reported stale
  baseline-browser-mapping data.
