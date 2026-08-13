## Why

OmniLLM has several independent reliability gaps in hot request, provider, and persistence paths: concurrent shutdown can panic, shared provider metadata can race, request bodies and imports can grow without bounds, canceled requests can remain queued, and database iteration failures can be cached or returned as successful partial data. These defects are difficult to reproduce individually but can cause crashes, memory pressure, stale routing, and misleading success under production concurrency or storage faults.

## What Changes

- Make asynchronous database worker shutdown safe against concurrent enqueue operations.
- Synchronize GitHub Copilot model-shape cache publication and lookup.
- Bound request bodies for Chat Completions, Responses, Messages, and token-counting endpoints.
- Bound uploaded configuration files and reject oversized payloads with an explicit HTTP 413 response.
- Make rate-limit waits request-context-aware so canceled requests stop waiting and do not retain reservations.
- Publish model-resolution cache snapshots only after complete, successful database reads.
- Publish provider-instance cache snapshots only after complete, successful database reads.
- Publish model-state cache snapshots only after complete, successful database reads.
- Propagate virtual-model and virtual-model-upstream row-iteration errors instead of returning partial results as success.
- Configure Knip with the repository's actual frontend, desktop, script, and test entrypoints so dead-code analysis is actionable rather than dominated by false positives.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-api`: Generation and token-counting request bodies have enforced size limits, oversized payloads receive HTTP 413, and queued rate-limit waits honor request cancellation.
- `admin-api`: Configuration imports have an enforced size limit and oversized uploads receive HTTP 413.
- `persistence`: Asynchronous writes remain safe during shutdown, database read failures are propagated, and incomplete routing/model-state snapshots are never published as loaded caches.
- `providers`: GitHub Copilot model-shape metadata supports safe concurrent refresh and lookup.
- `spec-governance`: Repository dead-code analysis uses maintained entrypoint configuration and completes without structural false positives.

## Impact

- Affected runtime code: request handlers, rate limiting, database async workers, persistence stores and caches, and GitHub Copilot model metadata.
- Affected tooling: Knip configuration and the `knip` package script.
- Observable API impact: oversized request bodies and config imports are rejected with HTTP 413; canceled queued requests return promptly.
- No database schema, external dependency, model-routing policy, or successful-request format changes.
