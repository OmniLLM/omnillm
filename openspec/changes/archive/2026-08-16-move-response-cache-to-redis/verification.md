# Redis response-cache verification

## Deterministic compatibility

The maintained compatibility route suites cover the affected Anthropic Messages and OpenAI Chat Completions paths with the shared five-exchange `AgenticScenario`, plus the Droid Responses custom-tool fixture as a non-cache regression. Verification command and result:

```text
go test ./internal/server -run 'TestCompatibilityAgenticRouteMatrix|TestCompatibilityRouteExecutesSuccessiveToolCycles|TestCompatibilityDroidCustomToolRoute' -count=1
ok omnillm/internal/server
```

The matrix maps the affected shapes to supported coding clients as follows:

- Claude Code: Anthropic Messages five-call/result history.
- GitHub Copilot CLI custom-provider mode: Chat Completions five-call/result history.
- Codex CLI: Responses five-call/result history (unchanged route), with existing native custom `exec` coverage in the compatibility suite.
- Droid: Responses five-call/result history plus sanitized `ApplyPatch` custom-tool association (unchanged route).

Response-cache route tests additionally verify Chat Completions and Messages fail open when cache storage is unavailable; the Responses route remains outside response-cache scope.

## Live client smoke availability

All four client executables are installed locally (`claude`, `codex`, `droid`, `copilot`), but a safe isolated live smoke also requires a configured OmniLLM provider/model route and its credential. `scripts/live-model-matrix.json` requires `OMNILLM_LIVE_GITHUB_TOKEN`; that variable is not set in this session. The existing `ANTHROPIC_BASE_URL`/`ANTHROPIC_AUTH_TOKEN` configure this active Claude session and are not sufficient to provision an isolated temporary OmniLLM upstream.

Therefore live client smokes are skipped for concrete missing configuration, not counted as passes:

- Claude Code: skipped — no isolated OmniLLM provider/model credential.
- Codex CLI: skipped — no isolated OmniLLM provider/model credential.
- Droid: skipped — no isolated OmniLLM provider/model credential.
- GitHub Copilot CLI custom-provider: skipped — `OMNILLM_LIVE_GITHUB_TOKEN` unavailable.

Deterministic client-shape regression coverage remains mandatory and was executed as recorded above.

## Real Redis and packaging evidence

A disposable `redis:8-alpine` container was bound to an ephemeral loopback port. The opt-in `TestRealRedisResponseCacheSmoke` passed under the race detector against that real server, covering bounded ping, save/miss/hit accounting, statistics, native expiration, namespace-scoped clear, and preservation of an unrelated key. Miniredis-backed route and store tests cover streaming replay, bypass/off controls, outage fast-fail, circuit recovery, malformed entries, and credential redaction.

```text
OMNILLM_TEST_REDIS_URL=redis://127.0.0.1:<ephemeral>/0 \
  go test -race ./internal/lib/responsecache -run TestRealRedisResponseCacheSmoke -count=1
ok omnillm/internal/lib/responsecache
```

The Tauri desktop application and OmniProxy sidecar built successfully. The Docker build initially exposed a pre-existing base-image mismatch (`golang:1.23-alpine` versus the module's Go 1.25 declaration); the Dockerfile now uses `golang:1.25-alpine`. A rebuild passed Go compilation and frontend production build, then stalled fetching the Alpine package index from `dl-cdn.alpinelinux.org` in the unchanged runner `apk add` step; it was stopped after the external mirror made no progress. `.dockerignore` now excludes the 5 GB Tauri target and sidecar output so Docker context is bounded.
