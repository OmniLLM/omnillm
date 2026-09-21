# TypeSafe provider verification

Human approval: the user instructed `proceed` after reviewing the strictly valid
proposal, delta specs, design, and task checklist. Implementation followed that
approval. Verification completed on 2026-09-21 (China Standard Time, UTC+8).

## Automated checks

- `bun run spec:validate`: 15 items passed, zero failed before implementation.
- `bun run spec:check`: strict validation and mandatory specification gate passed.
- `bun run lint:all`: passed; only the existing browser-mapping age notice.
- `bun run typecheck`: passed.
- `bun test`: 372 passed, 22 skipped, zero failed; includes rendered TypeSafe
  password-form coverage and evaluation-model exclusion from Chat selection.
- `bun run build`: passed; existing chunk-size advisory remains.
- `go vet ./...` and `go build ./...`: passed.
- `go test -race ./...`: passed across the full repository.
- After final tests and UI refinements, the affected provider, System One,
  server, routes, CLI, dispatch, and routing packages passed again under `-race`.
- `git diff --check`: passed. All changed/untracked files were scanned against
  the supplied TypeSafe credential; no credential matches were found.

Coverage includes exact native request/response preservation for Noul, Choice,
and Score; upstream catalog parsing; credential persistence and failed
re-authentication; malformed requests/responses; redirect rejection; bounded
requests and caller cancellation; inbound auth and concurrency ordering;
provider qualification and version pinning; disabled model filtering;
virtual-model operation filtering; failover and Retry-After preservation;
metering; all three generation dialects and embeddings rejection; environment
credential precedence; and existing five-turn coding-agent client fixtures.

The expanded suite exposed an existing catalog test double that implemented
GetID/GetInstanceID but inherited GetName from a nil embedded Provider and left
itself registered. Subsequent provider persistence panicked. Its name method
and cleanup are now complete; no production registry behavior was changed.
The existing CLI catalog expectation was updated for the additional provider.

## TypeSafe live evidence

Command: `OMNILLM_TYPESAFE_LIVE=1 go test ./internal/server -run
'^TestTypeSafeLiveSmoke$' -v -timeout 90s`, with TYPESAFE_API_KEY exported from the
operator-supplied file without shell tracing. The ordinary test suite skips this
credential-gated smoke unless explicitly enabled.

The smoke used a temporary database and fresh gateway port **51970**. It verified
API-key authentication, reconstruction from persisted credentials, native model
listing, and a mixed-primitive evaluation through `/v1/systemone`:

- HTTP 200, resolved model `jev-1.13.0`.
- Answers retained their Noul, Choice, and Score types and native detail.
- Upstream usage: 373 input tokens and 62 output tokens.
- Gateway model listing exposed evaluation-only `systemone` metadata.

The first smoke attempt incorrectly used provider deletion to simulate restart;
that correctly deleted its token. The smoke was corrected to reconstruct a fresh
provider from durable state. The provider's persistent-load unit test and final
live evaluation passed. This was a test-harness error, not a TypeSafe API failure.

## Coding-client live compatibility

All four installed coding clients ran on an isolated gateway at fresh loopback
port **53824** using the configured Copilot `gpt-6-astra` model. Only the selected
provider credential was copied into disposable gateway state. Each client used
isolated settings and working directories. Normal OmniLLM runtime state and
client settings were not modified. Temporary credential-bearing state was removed
when verification finished.

| Client | Calls | Results | Sequential/matching IDs | Terminal | Exit |
| --- | ---: | ---: | --- | --- | ---: |
| Codex CLI | 5 | 5 | yes | TYPESAFE_CLIENT_OK | 0 |
| Claude Code | 5 | 5 | yes | TYPESAFE_CLIENT_OK | 0 |
| Droid | 5 | 5 | yes | TYPESAFE_CLIENT_OK | 0 |
| GitHub Copilot CLI custom provider | 5 | 5 | yes | TYPESAFE_CLIENT_OK | 0 |

JSON event transcripts were checked for alternating call/result events, five
unique matching identifiers, successful results, and terminal completion.
Sanitized evidence is in `client-verification.json`. TypeSafe itself is explicitly
not applicable to native tool loops: Jev exposes typed evaluation only.

## Scope and operational state

No SDK dependency, migration, deployment, or normal-account auto-registration was
introduced. TypeSafe is available after building this change and explicitly
registering its key through CLI or administration UI. Native evaluations bypass
generation CIF and exact-response caching. CLI help and README explain credential
loading, routing, version pinning, metering, and client-side backoff.

Archive completed through OpenSpec; post-archive `bun run spec:check` passed
(14 current-state specs and the mandatory gate). Only surplus trailing blank
lines emitted by the archive tool were normalized afterward.

## Pre-ship verification

OpenSpec validation/gate, lint, typecheck, all Bun tests, frontend build, Go vet,
and Go build passed again. The first full race-test run intermittently failed
`TestAnthropicMessagesRouteStripsPrefixedVirtualModelUpstreamID` and
`TestProviderPrefixRouting_SubtitleResolution`. Both passed three focused
repetitions, and the subsequent complete `go test -race ./...` passed without
code changes. An unchanged `ace041d` checkout passed twenty focused repetitions
and its full server suite; the initial failures were not reproduced there, so
their cause remains unconfirmed. The PR must also pass remote CI before merging.
The changed-file scan found no matches for the supplied TypeSafe credential.
