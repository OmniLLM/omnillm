---
name: log-triage
description: Continuously improve OmniLLM by mining ~/.omnillm/omnillm.log for errors, latency outliers, provider failures, and anomalies, then tracing them to root cause in the Go/TS codebase and proposing or applying fixes. Use when asked to "check the logs", "find bugs from logs", "improve the project", or during periodic health reviews.
---

# OmniLLM Log Triage & Improvement

Goal: turn runtime evidence in `~/.omnillm/omnillm.log` into concrete, verified code improvements.

## 1. Gather evidence

Log is JSON-lines (zerolog). Key fields: `level`, `request_id`, `api_shape`,
`model_requested`, `model_used`, `provider`, `stop_reason`, `stream`,
`input_tokens`, `output_tokens`, `latency_ms`, `status`, `path`, `message`.

Start with these passes (adjust window as needed):

```bash
LOG=~/.omnillm/omnillm.log

# 1. Non-info levels — the primary bug signal
grep -h '"level":"\(warn\|error\|fatal\|panic\)"' $LOG* | tail -200

# 2. HTTP failures grouped by status + path
jq -r 'select(.message=="HTTP" and .status>=400) | "\(.status) \(.path)"' $LOG | sort | uniq -c | sort -rn

# 3. Failures by provider / model
jq -r 'select(.level=="error") | "\(.provider // "-") \(.model_requested // "-") \(.message)"' $LOG | sort | uniq -c | sort -rn

# 4. Latency outliers
jq -r 'select(.latency_ms!=null) | [.latency_ms, .provider, .model_used, .path] | @tsv' $LOG | sort -rn | head -30

# 5. Requests with no matching RESPONSE (hangs / dropped streams)
jq -r 'select(.message|test("REQUEST|RESPONSE")) | "\(.request_id) \(.message)"' $LOG \
  | awk '{c[$1]++} END{for(r in c) if(c[r]<2) print r}'

# 6. Token/context anomalies (near-limit inputs, zero-output responses)
jq -r 'select(.output_tokens==0 or .input_tokens>180000) | [.request_id,.model_used,.input_tokens,.output_tokens] | @tsv' $LOG
```

Also check rotated files (`omnillm.log.*`) and `.omni-dev.log` in the repo root.

## 2. Triage

For each distinct signal produce: symptom → frequency → affected
provider/route → hypothesis. Rank by (frequency × severity). Ignore
one-off client disconnects unless recurring.

Prefer a handful of well-understood issues over a long shallow list.

## 3. Trace to code

Map the signal to the owning package before reading broadly:

- `internal/providers/`, `internal/providerdispatch/` — upstream calls, auth, token refresh, retries
- `internal/routes/`, `internal/server/` — HTTP handlers, status codes, streaming
- `internal/serialization/` — request/response shape translation (anthropic ↔ openai ↔ gemini)
- `internal/registry/` — model aliasing (`owner/model` → `model_used`)
- `internal/services/`, `internal/database/`, `internal/ingestion/`, `internal/security/`
- `frontend/`, `desktop/` — UI-side issues

Grep the exact log `message` string to find the emitting site, then read
outward from there.

## 4. Fix

- One issue per change; smallest correct fix.
- Add a regression test under `tests/` or the package's `_test.go` whenever
  the bug is reproducible.
- If the log lacked the detail needed to diagnose, **improving the log line
  itself is a valid fix** (add provider, status code, upstream error body,
  retry count, request_id propagation).

Verify:

```bash
make lint && make typecheck && make test
```

## 5. Report

Summarize as a table: issue | evidence (count + sample request_id) | root cause | fix | test.
List anything found but *not* fixed as follow-ups with rationale.

Never invent log evidence — if a pass returns nothing, say so.

## Beyond logs — other improvement angles

Rotate through these when the log is quiet:

1. **Observability gaps** — requests that error with no actionable field; missing
   metrics on retries, cache hits, queue depth, per-provider cost.
2. **Reliability** — retry/backoff policy, timeouts, circuit breaking on a
   failing provider, graceful stream cancellation, token-refresh races.
3. **Performance** — latency outliers by provider, TTFT for streams, allocation
   hot spots, N+1 DB queries in `internal/database`.
4. **Security** — API keys or bearer tokens leaking into logs, request/response
   body logging with user content, path traversal, CORS, auth on admin routes.
   (`mcp__plugin_aikido_aikido-mcp__aikido_full_scan` can scan changed files.)
5. **Correctness of shape translation** — round-trip tests for
   anthropic/openai/gemini conversions, tool-call and multimodal edge cases,
   `stop_reason` mapping.
6. **Test coverage** — `go test ./... -cover`, target packages under ~60%.
7. **Config & DX** — undocumented env vars, confusing errors on misconfig,
   README drift vs actual flags/targets.
8. **Dependency health** — `go list -u -m all`, `bun outdated`, known CVEs.
9. **Log hygiene** — is the log itself rotated/size-bounded? Is verbosity
   configurable? Are ANSI escape codes in `message` (they are today) hurting
   machine parsing?

## Conventions

- Commit as `James Zhu <zhujian0805@gmail.com>`; no `Co-Authored-By` lines.
- Branch before committing if on the default branch.
