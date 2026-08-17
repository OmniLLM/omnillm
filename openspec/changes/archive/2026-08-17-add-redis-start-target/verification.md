# Redis Make startup verification

## Target behavior

`make help` lists `start-redis`, the canonical `OMNILLM_RESPONSE_CACHE_REDIS_URL` environment variable, the backend default `redis://127.0.0.1:6379/0`, shell-labeled POSIX, PowerShell, and `cmd.exe` custom-endpoint examples, and the separate response-cache enable command.

The target is a shell-neutral alias of the existing managed background startup workflow:

```text
make --dry-run start-redis
bun run omni start --server-port 5000 --frontend-port 5080 --host 127.0.0.1

make --dry-run OS=Windows_NT start-redis
bun run omni start --server-port 5000 --frontend-port 5080 --host 127.0.0.1
```

A fake Bun executable inspected its inherited environment while Make received a credential-bearing Redis URL containing literal `$`, parentheses, backticks, a double quote, `%`, and `&`. The child received the exact value, and no injected file or command was produced. This confirms that neither Make nor its recipe shell reparses the Redis URL.

## Required checks

The following checks passed after the secure alias revision:

```text
bun run spec:check
bun run lint:all
bun run typecheck
bun test
bun run build
go vet ./...
go build ./...
go test -race ./...
git diff --check
```

Results included strict validation of all 15 OpenSpec items, a passing mandatory spec gate, 358 passing and 22 skipped Bun tests with zero failures, and passing race-enabled Go tests across all packages. The skipped Bun tests were the existing opt-in dangerous visual suite and server-dependent integration path. Frontend build emitted only the existing chunk-size advisory; lint emitted only the existing stale baseline-browser-mapping advisory.

## Review

An independent focused review initially identified unsafe shell interpolation and Windows-shell selection in the first implementation. That implementation was fully reverted. The reviewer rechecked the final `start-redis: start` alias, adversarial environment test, updated OpenSpec contract, and full verification results and reported no remaining actionable high-confidence issues.
