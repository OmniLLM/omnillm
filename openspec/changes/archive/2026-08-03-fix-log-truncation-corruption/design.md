# Design: Encoding-safe, non-mutating log truncation

## Context

Two helpers truncate values before they reach a log field:

- `cappedBody([]byte) []byte` in `internal/providers/openaicompat/http.go`, limit 1024, used at four trace call sites.
- `truncateToolLoopValue(string) string` in `internal/routes/tool_loop_logging.go`, limit 400, used at four preview sites.

Both are wrong, for different reasons.

## The aliasing bug

```go
return append(b[:traceBodyLimit], []byte("...(truncated)")...)
```

`b[:traceBodyLimit]` shares the backing array with `b` and inherits its capacity. When `cap(b) > traceBodyLimit`, `append` has room and writes in place, so the marker overwrites bytes `[1024:1038]` of the caller's slice rather than producing a copy.

The call ordering makes this a wire-level defect, not a cosmetic one:

```go
log.Trace()....RawJSON("payload", cappedBody(body))   // line 137: mutates body
req, err := newPOSTRequest(ctx, url, headers, body, false)  // line 140: sends mutated body
```

`Marshal` returns a freshly allocated slice whose capacity typically exceeds its length, so the aliasing condition is met in practice. Every openaicompat request over 1024 bytes is silently malformed while trace logging is enabled.

This also explains why the bug survived: the log line renders the *intended* truncated text, so the log gives no hint that the outbound body was damaged. Only reading the helper reveals it.

Fix: allocate explicitly rather than relying on `append` growth.

```go
out := make([]byte, 0, limit+len(marker))
out = append(out, b[:limit]...)
out = append(out, marker...)
```

Rejected alternative: reorder the call sites so logging happens after the request is built. That would fix these four sites while leaving a booby-trapped helper for the next caller. Fixing the helper is the durable fix.

## The encoding bug

Both helpers slice at a fixed byte offset. A multi-byte rune straddling that offset is severed, leaving orphaned continuation bytes and an invalid-UTF-8 log field. `RawJSON` and `Str` do not repair this.

Fix: back off to the nearest rune boundary at or below the limit, using `utf8.RuneStart` / `utf8.DecodeLastRune`. Backing off never exceeds the limit, so the byte budget remains a hard ceiling.

Applied to `cappedBody` too. Its payloads are JSON and therefore usually ASCII, but they carry user content that can be any UTF-8, and `RawJSON` on a severed rune produces a malformed field.

Note the two helpers keep separate limits and separate signatures (`[]byte` vs `string`) rather than being merged. They live in different packages with different call conventions; a shared generic helper would add a dependency edge for ~10 lines of logic.

## Non-goals

- Changing limits (1024 / 400) or the `...(truncated)` marker.
- Reducing tool-loop log verbosity or redacting `tool_result` content. That is a separate policy question, tracked as a follow-up.
- Touching log rotation, which is already correctly configured via lumberjack (50 MB, 3 backups, 28 days, compressed).

## Risks

Very low. Both changes are strictly more conservative: output is never longer than before, never aliases the input, and differs only where the previous output was already malformed.
