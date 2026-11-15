## Verification

- Focused CLI, metadata sanitization, admin API, and admin authentication tests passed under the race detector. Synthetic credentials and URL components were checked for exclusion from responses, output, errors, and logs.
- Tests cover available and missing OAuth metadata, provider reference ambiguity, active OAuth flow data, configured endpoint origins, malformed credential records, old-server fallback, and metadata request errors.
- `bun run spec:check`, `bun run lint:all`, `go vet ./...`, `go build ./...`, `go test -race ./...`, and `git diff --check` passed.
- No production server or credential data was used for the tests. The change has not been installed or deployed.
