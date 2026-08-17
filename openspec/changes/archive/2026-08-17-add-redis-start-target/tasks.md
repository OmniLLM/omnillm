## 1. Contract and Validation

- [x] 1.1 Run `bun run spec:validate` and obtain human approval for the initial proposal, delta spec, design, and tasks before implementation.
- [x] 1.2 Revalidate and obtain human approval for the revised inherited-environment design before reimplementation.

## 2. Make Workflow

- [x] 2.1 Add the phony `start-redis` target as a shell-neutral alias of the existing managed background-service startup workflow.
- [x] 2.2 Add the canonical Redis URL environment variable, local default, target, and runnable examples to `make help`.

## 3. Verification

- [x] 3.1 Verify `make help` exposes the target, canonical environment variable, local default, and examples.
- [x] 3.2 Verify the target's default dry run on POSIX and Windows Make branches and use a fake Bun executable to confirm a credential-bearing inherited Redis URL reaches the child unchanged without shell evaluation.
- [x] 3.3 Run `bun run spec:check`, `bun run lint:all`, `bun run typecheck`, `bun test`, `bun run build`, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 3.4 Record verification results and archive the change only after every task passes.
