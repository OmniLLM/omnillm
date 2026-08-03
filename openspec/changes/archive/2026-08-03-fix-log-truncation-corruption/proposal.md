## Why

Two defects in log-truncation helpers, both found by reading the truncation paths after a log-triage pass.

**1. Trace logging corrupts the request body actually sent upstream (severe).**

`cappedBody` in `internal/providers/openaicompat/http.go` truncates with `append(b[:traceBodyLimit], "...(truncated)"...)`. Because `b[:limit]` retains the original array's capacity, the `append` writes the suffix *into the caller's buffer*, overwriting live request bytes. Verified:

```
before: AAAAAAAAAABBBBBBBBBBCCCCCCCCCC
after : AAAAAAAAAA...(truncated)CCCCCC
```

At `http.go:137` the corrupting call runs immediately before `newPOSTRequest(ctx, url, headers, body, false)` at line 140, so the mutated buffer is what gets transmitted. Any openaicompat request larger than 1024 bytes is silently malformed whenever trace logging is on, across all four call sites in `http.go` and `responses.go`. The failure is invisible in the log itself, because the log shows the truncated string that was intended.

**2. Tool-loop previews emit invalid UTF-8 (cosmetic but real).**

`truncateToolLoopValue` slices at a fixed 400-*byte* offset with no rune awareness, splitting any multi-byte character straddling the boundary. Verified to produce a trailing partial rune (`... e4` with the continuation bytes lost). Non-ASCII tool results — CJK, emoji, accented text — yield log entries that are not valid UTF-8, which breaks strict JSON-log consumers.

## What Changes

- Make `cappedBody` allocate a new buffer so truncation for logging can never mutate or alias the caller's data.
- Make both truncation helpers rune-aware, cutting at a UTF-8 boundary at or below the configured limit.
- Preserve existing limits (1024 bytes trace body, 400 bytes tool-loop preview) and the existing `...(truncated)` marker.
- Add regression tests proving the caller's buffer is unchanged after truncation and that output is always valid UTF-8.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providers`: diagnostic truncation is non-mutating and encoding-safe.
- `gateway-api`: truncated log previews are always valid UTF-8.

## Impact

- Affects `internal/providers/openaicompat/http.go` and `internal/routes/tool_loop_logging.go`.
- Fixes silent upstream request corruption under trace logging — a correctness bug reaching the wire, not merely a logging defect.
- No API shape, routing, timeout, retry, or persistence behavior changes. Log limits and markers are unchanged.
