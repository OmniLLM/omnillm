## Context

The existing `start` Make target invokes the Bun-based service manager, which starts both the compiled Go backend and Vite frontend in the background. The manager and Make recipes inherit their process environment, and the backend already resolves `OMNILLM_RESPONSE_CACHE_REDIS_URL` when supplied or uses `redis://127.0.0.1:6379/0` by default. See `proposal.md` for motivation and `specs/cli-ops-config/spec.md` for the observable contract.

Redis URLs can contain credentials and characters that recipe shells interpret. The Make workflow must therefore preserve the canonical environment value without expanding it into recipe shell source, and it must work regardless of whether GNU Make uses a POSIX shell, `cmd.exe`, or another Windows shell.

## Goals / Non-Goals

**Goals:**

- Preserve the same managed-service startup path used by `make start`.
- Preserve the canonical Redis environment value as inherited process data rather than recipe shell syntax.
- Keep the backend's local default convenient and allow operators to supply authenticated, TLS, remote, or non-default database URLs through the existing environment variable.
- Keep the target discoverable through the current hand-authored help output.

**Non-Goals:**

- Starting, stopping, or provisioning the Redis container.
- Automatically enabling the durable `response_cache.enabled` setting.
- Changing the Bun manager CLI, backend CLI, Redis lifecycle, or cache failure behavior.
- Adding Redis configuration to restart or foreground development targets.

## Decisions

### Add a dedicated `start-redis` alias

`start-redis` will depend on the existing `start` target. GNU Make propagates the caller's environment to the existing recipe, so the backend receives `OMNILLM_RESPONSE_CACHE_REDIS_URL` unchanged through the service manager without placing its value in a recipe. If the variable is absent, the backend retains its existing local default.

Alternative considered: modify `start` or construct a shell command that assigns the Redis URL. Rejected because a dedicated alias makes operator intent visible, leaves existing startup behavior untouched, and avoids parsing credential-bearing URLs as POSIX or Windows shell syntax.

Alternative considered: define `REDIS_URL` and use a target-specific exported Make assignment. Rejected because Make command-line variable syntax still gives Make special interpretation to `$`, while the canonical backend environment variable already supports arbitrary inherited values and requires no forwarding logic.

Alternative considered: add a service-manager CLI option and forward it to the backend. Rejected because this small operator workflow does not require expanding another public CLI surface or touching multiple implementation files.

### Document the canonical environment variable and backend default

Help will display `OMNILLM_RESPONSE_CACHE_REDIS_URL`, its backend default `redis://127.0.0.1:6379/0`, and examples for default and custom endpoints. A custom value is supplied in the process environment before invoking Make, for example `OMNILLM_RESPONSE_CACHE_REDIS_URL='rediss://user:password@host:6380/0' make start-redis` on POSIX systems. Because Make does not interpolate that value into a recipe, shell interpretation occurs only once when the operator intentionally creates their environment assignment.

Alternative considered: expose the response-cache namespace prefix. Rejected because the request concerns connecting to the running Redis service; the existing `omnillm` prefix remains suitable and can still be configured through the backend's canonical prefix environment variable.

## Risks / Trade-offs

- [The target connects Redis but does not turn on response caching] → Keep this distinction explicit in the help output and provide the existing `omnillm settings set response-cache on --ttl 3600` follow-up command.
- [An operator's shell may interpret metacharacters while they establish an environment variable] → Examples quote the value where applicable; Make and its recipes never reinterpret or print the credential-bearing value.
- [The alias may appear redundant because the backend already defaults to local Redis] → Keep it as a discoverable intent-oriented workflow paired with the enabling command, while delegating all runtime behavior to the existing tested `start` target.
- [Windows environment-assignment syntax varies by shell] → Do not generate an assignment in a recipe; inherit the canonical environment in a shell-neutral dependency alias.

## Migration Plan

No migration is required. Operators may continue using `make start`; `make start-redis` is additive. Rollback consists of removing the new target, variable, platform-specific command prefix, and help entries.
