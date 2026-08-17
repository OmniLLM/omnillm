## Why

OmniLLM can use an already-running Redis service for exact-response caching, but the repository Make workflow does not expose the Redis URL when starting the managed backend and frontend services. Operators currently have to bypass the documented `make start` workflow or discover how to forward backend configuration themselves.

## What Changes

- Add a Make target that starts OmniLLM's managed services using the backend's existing response-cache Redis URL resolution.
- Preserve the backend's local Redis default and inherited `OMNILLM_RESPONSE_CACHE_REDIS_URL` override without interpolating Redis credentials into recipe shell source.
- List the target and canonical environment variable in `make help` and include runnable examples.
- Preserve the existing `start` target behavior and the server's fail-open Redis semantics.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-ops-config`: Extend the repository's operator startup workflow with a discoverable Redis-enabled Make target that forwards the configured Redis endpoint to OmniLLM.

## Impact

- Affected code: `Makefile`.
- Affected behavior: local/operator startup through Make only; server CLI flags, APIs, cache semantics, dependencies, and persisted data formats are unchanged.
- No breaking changes and no new dependencies.
