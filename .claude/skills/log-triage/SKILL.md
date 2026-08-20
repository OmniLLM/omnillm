---
name: log-triage
description: Mine OmniLLM and Claude Code logs from the past two days by default, correlate runtime failures, and determine which issues are fixable in the OmniLLM codebase. Use when asked to "check the logs", "find bugs from logs", "improve the project", or during periodic health reviews.
---

# OmniLLM Log Triage & Improvement

Goal: turn correlated runtime evidence from OmniLLM and Claude Code into actionable findings and, when requested, verified code improvements, while separating OmniLLM defects from client, plugin, configuration, and environment issues.

## 1. Gather evidence

Unless the user gives another window, inspect the **past 2 days**. Apply the time filter before counting or sampling so older incidents do not distort the result. Report the requested window and the actual timestamp range present in each log.

### OmniLLM logs

Primary sources:

- `~/.omnillm/omnillm.log` and rotated `omnillm.log.*`
- `.omni-dev.log` in the repository root

The OmniLLM log is JSON-lines (zerolog). Key fields: `level`, `request_id`,
`api_shape`, `model_requested`, `model_used`, `provider`, `stop_reason`,
`stream`, `input_tokens`, `output_tokens`, `latency_ms`, `status`, `path`,
`message`, and the timestamp field (`time` or `timestamp`).

Filter JSON entries to the window first, then run these passes over the filtered data:

1. Non-info levels (`warn`, `error`, `fatal`, `panic`).
2. HTTP failures grouped by status and path.
3. Failures grouped by provider, requested model, and message.
4. Latency and time-to-first-token outliers.
5. Request IDs with an initial request but no terminal response; exclude completed retries and expected stream cancellations.
6. Token/context anomalies such as near-limit input or zero output.
7. Repeated retry, timeout, serialization, upstream-body, or stream errors.

Use a small Python script when timestamp filtering or multiline correlation would make a `grep | jq` pipeline misleading. Never assume rotated files are ordered or share the same schema.

### Claude Code log

Also inspect `~/tmp/claude.log` by default. It is timestamped plain text, normally shaped like:

```text
2026-08-04T10:11:38.129Z [ERROR] API error (attempt 1/11): undefined Request timed out.
```

Filter by the leading ISO-8601 timestamp to the same 2-day window, then:

1. Count lines by level and normalize repeated messages by replacing volatile request IDs, durations, process IDs, and filesystem paths; preserve HTTP routes and status codes.
2. Extract API failures, slow-first-byte warnings, retries, stream interruptions, malformed responses, timeouts, and HTTP status codes.
3. Correlate nearby `[API REQUEST]`, dispatch model, timing, retry, and completion lines into incidents. Do not count every retry line as a separate user-visible failure.
4. Treat tool execution failures, missing plugin credentials, unauthorized MCP servers, marketplace path errors, and missing executables as Claude Code/plugin/environment issues unless OmniLLM evidence shows the model request caused them.
5. Use model/provider/request timing and matching timestamps to correlate a Claude Code symptom with `~/.omnillm/omnillm.log`. Prefer a shared request ID; otherwise label timestamp/model correlation as circumstantial.
6. Redact secrets, bearer tokens, user prompt content, and tool payloads from samples.

Claude Code may log high-volume DEBUG noise. Prioritize WARN/ERROR plus API lifecycle context rather than dumping all matching lines.

## 2. Triage and ownership

For each distinct signal produce: symptom → incident count (not raw retry-line count) → affected provider/route/model → evidence → hypothesis → ownership verdict. Rank by severity, recurrence, and user impact. Ignore one-off client disconnects unless recurring.

Assign exactly one ownership verdict to each incident:

- **OmniLLM-fixable** — evidence points to provider dispatch, translation, routing, retries, streaming, registry, persistence, security, or observability code in this repository.
- **Possibly OmniLLM** — correlation exists but lacks a request ID, upstream response, or sufficient diagnostic fields; state what evidence would confirm it. Separately note any OmniLLM observability improvement that would make the incident diagnosable.
- **Not OmniLLM** — Claude Code, MCP/plugin configuration, credentials, local environment, external upstream outage, or user command failure.
- **Expected/transient** — successful retry, cancellation, or harmless startup noise with no recurring or material impact.

A Claude Code `[ERROR]` line is not by itself proof of an OmniLLM defect. Check whether retries exhausted, whether the request later succeeded, and whether OmniLLM recorded a matching failure. A recovered retry with recurring or material reliability/latency impact should not be classified as expected/transient.

Prefer a handful of well-understood issues over a long shallow list.

## 3. Trace to code

Map the signal to the owning package before reading broadly:

- `internal/providers/`, `internal/providerdispatch/` — upstream calls, auth, token refresh, retries
- `internal/routes/`, `internal/server/` — HTTP handlers, status codes, streaming
- `internal/serialization/` — request/response shape translation (anthropic ↔ openai ↔ gemini)
- `internal/registry/` — model aliasing (`owner/model` → `model_used`)
- `internal/services/`, `internal/database/`, `internal/ingestion/`, `internal/security/`
- `frontend/`, `desktop/` — UI-side issues

Search for a stable, redacted fragment of the log `message` to find the emitting site, then read outward from there.

## 4. Fix when requested

Only implement after the repository's OpenSpec proposal is validated and approved.

- One issue per change; smallest correct fix.
- Add a regression test under `tests/` or the package's `_test.go` whenever
  the bug is reproducible.
- If the log lacked the detail needed to diagnose, **improving the log line
  itself is a valid fix** (add provider, status code, upstream error body,
  retry count, request_id propagation).

Verify:

```bash
bun run lint:all && bun run typecheck && bun test
```

## 5. Report

Summarize as a table:

| issue | evidence (incident count + redacted sample/request ID) | correlation | ownership | root cause or hypothesis | recommended fix/test |
|---|---|---|---|---|---|

Separate these outcomes clearly:

1. OmniLLM fixes worth proposing.
2. Possible OmniLLM issues needing better evidence or logging.
3. Claude Code/plugin/environment issues that should not change this repository.
4. Expected or transient events requiring no action.

List unproposed or unapplied findings as follow-ups with rationale. If implementation is requested, follow the repository's OpenSpec workflow first. Pure log analysis and edits limited to this skill are documentation-only.

Never invent log evidence. If a pass returns nothing, say so. State when the log contains less than the requested window.

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
