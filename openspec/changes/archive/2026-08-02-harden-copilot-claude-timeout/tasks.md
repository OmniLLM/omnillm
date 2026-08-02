## 1. Specification and regression coverage

- [x] 1.1 Correlate timeout log records by request, provider, model, route, and elapsed duration.
- [x] 1.2 Define the provider timeout and diagnostic behavior in a strict OpenSpec delta.
- [x] 1.3 Add regression tests for Claude client selection, ordinary-client preservation, and timeout context logging.

## 2. Copilot timeout implementation

- [x] 2.1 Add a dedicated configurable Copilot Claude timeout client using the shared production transport.
- [x] 2.2 Select the Claude client for Claude chat-completions header acquisition without changing established SSE body handling.
- [x] 2.3 Emit structured timeout diagnostics with request, provider, model, endpoint, budget, and elapsed context.

## 3. Verification

- [x] 3.1 Run strict OpenSpec validation and the specification gate.
- [x] 3.2 Run focused Copilot tests, `go test -race ./internal/providers/copilot`, `go vet ./...`, and `go build ./...`.
- [x] 3.3 Run the repository Bun governance tests and archive the completed change.
