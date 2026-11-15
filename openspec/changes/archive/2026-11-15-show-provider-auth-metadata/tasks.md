## 1. Specification and test design

- [x] 1.1 Strictly validate the change artifacts and obtain human approval before implementation.
- [x] 1.2 Add deterministic tests for provider metadata authentication, reference resolution, available and absent fields, and secret exclusion from API responses, logs, errors, and both CLI output formats.
- [x] 1.3 Add tests for URL sanitization, active OAuth flows, older-server compatibility, and metadata read failures.

## 2. Server metadata

- [x] 2.1 Implement the authenticated read-only provider metadata operation using explicit allowlists and existing provider lifecycle protection.
- [x] 2.2 Provide stable OAuth endpoints and public client IDs from authoritative provider data, and safely extract applicable configured metadata without exposing credential records.

## 3. CLI and documentation

- [x] 3.1 Extend `provider show` table and JSON output with safe metadata, preserving existing reference and model inspection behavior.
- [x] 3.2 Handle older servers and errors explicitly; document available fields, redaction, and the difference between stable endpoints and temporary login URLs.

## 4. Verification

- [x] 4.1 Run focused CLI, admin API, and security tests including synthetic secret sentinels.
- [x] 4.2 Run `bun run spec:check`, `go vet ./...`, `go build ./...`, and `go test -race ./...`; run other relevant checks and record limitations.
