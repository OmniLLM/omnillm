# Tasks

- [x] 1. Add failing OpenAI provider tests for nested token errors, recovery
  from a newer durable refresh token, terminal reauthentication persistence,
  and bounded concurrent refresh behavior.
- [x] 2. Implement sanitized OpenAI token-error decoding and a typed rotation
  rejection signal.
- [x] 3. Implement one-shot recovery from newer durable credentials and durable
  retirement of a rejected refresh token.
- [x] 4. Run `bun run spec:validate` and confirm the change remains strict.
- [x] 5. Run `go test -race -count=1 ./internal/providers/openai`.
- [x] 6. Run `go test -race -count=1 ./...`, `go vet ./...`, and
  `bun run spec:check`.
